// Kube-tagged VM reconciliation (ADR-0045): upsert + status computation.
package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/sthalbert/longue-vue/internal/api"
)

const kubeNodeVMColumns = `k.id, k.cloud_account_id, ca.name, k.provider_vm_id, k.cluster_tag, k.node_name_tag,
	k.cluster_hint, k.name, k.instance_type, k.power_state, k.zone, k.vpc_id, k.image_id, k.image_name,
	k.provider_creation_date, k.node_id, k.cluster_id, c.name, k.status, k.status_since, k.first_seen_at, k.last_seen_at`

const kubeNodeVMFrom = ` FROM kube_node_vms k
	JOIN cloud_accounts ca ON ca.id = k.cloud_account_id
	LEFT JOIN clusters c ON c.id = k.cluster_id`

// ReconcileKubeNodeVMs upserts the per-tick set of kube-tagged VMs of one
// account, deletes rows absent from items, then recomputes every status of
// the account in one transaction (ADR-0045 §3).
//
//nolint:gocyclo // upsert loop + delete + status-transition tally; splitting would obscure the single transactional flow
func (p *PG) ReconcileKubeNodeVMs(
	ctx context.Context,
	accountID uuid.UUID,
	items []api.NodeImage,
	grace time.Duration,
) (api.KubeNodeVMReconcileResult, error) {
	var res api.KubeNodeVMReconcileResult
	const upsert = `
		INSERT INTO kube_node_vms (cloud_account_id, provider_vm_id, cluster_tag, node_name_tag, cluster_hint,
			name, instance_type, power_state, zone, vpc_id, image_id, image_name, provider_creation_date, last_seen_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,now())
		ON CONFLICT (cloud_account_id, provider_vm_id) DO UPDATE SET
			cluster_tag = EXCLUDED.cluster_tag, node_name_tag = EXCLUDED.node_name_tag,
			cluster_hint = EXCLUDED.cluster_hint, name = EXCLUDED.name,
			instance_type = EXCLUDED.instance_type, power_state = EXCLUDED.power_state,
			zone = EXCLUDED.zone, vpc_id = EXCLUDED.vpc_id,
			image_id = EXCLUDED.image_id, image_name = EXCLUDED.image_name,
			provider_creation_date = COALESCE(EXCLUDED.provider_creation_date, kube_node_vms.provider_creation_date),
			last_seen_at = now()`
	const del = `DELETE FROM kube_node_vms WHERE cloud_account_id = $1 AND NOT (provider_vm_id = ANY($2))`
	// Status computation. "matched" joins live nodes by provider_id suffix
	// (exact, no LIKE). "known" is the set of (hint, tag, cluster_id) that at
	// least one matched sibling proves. RETURNING exposes old and new status
	// so the caller can log transitions.
	const compute = `
		WITH matched AS (
			SELECT k.id AS kid, n.id AS nid, n.cluster_id
			  FROM kube_node_vms k
			  JOIN nodes n ON n.terminated_at IS NULL
			   AND right(n.provider_id, length(k.provider_vm_id) + 1) = '/' || k.provider_vm_id
			 WHERE k.cloud_account_id = $1
		), known AS (
			SELECT DISTINCT k.cluster_hint, k.cluster_tag, m.cluster_id
			  FROM kube_node_vms k JOIN matched m ON m.kid = k.id
		), calc AS (
			SELECT k.id, k.status AS old_status, m.nid,
			       CASE
			         WHEN m.nid IS NOT NULL THEN 'node'
			         WHEN COALESCE(k.provider_creation_date, k.first_seen_at) > now() - $2::interval THEN 'pending'
			         WHEN EXISTS (SELECT 1 FROM known w
			                       WHERE (k.cluster_hint <> '' AND w.cluster_hint = k.cluster_hint)
			                          OR (k.cluster_tag <> '' AND w.cluster_tag = k.cluster_tag)) THEN 'orphan'
			         ELSE 'unknown_cluster'
			       END AS new_status,
			       COALESCE(m.cluster_id, (SELECT w.cluster_id FROM known w
			                                WHERE (k.cluster_hint <> '' AND w.cluster_hint = k.cluster_hint)
			                                   OR (k.cluster_tag <> '' AND w.cluster_tag = k.cluster_tag)
			                                LIMIT 1)) AS new_cluster_id
			  FROM kube_node_vms k LEFT JOIN matched m ON m.kid = k.id
			 WHERE k.cloud_account_id = $1
		)
		UPDATE kube_node_vms k
		   SET node_id = c.nid, cluster_id = c.new_cluster_id,
		       status_since = CASE WHEN k.status <> c.new_status THEN now() ELSE k.status_since END,
		       status = c.new_status
		  FROM calc c
		 WHERE c.id = k.id
		RETURNING k.provider_vm_id, c.old_status, c.new_status`

	err := p.withTx(ctx, "reconcile kube node vms", func(tx pgx.Tx) error {
		keep := make([]string, 0, len(items))
		for i := range items {
			it := items[i]
			if !validProviderVMID(it.ProviderVMID) {
				continue // defense in depth; skip malformed ids
			}
			if _, e := tx.Exec(ctx, upsert, accountID, it.ProviderVMID, it.ClusterTag, it.NodeNameTag, it.ClusterHint,
				it.Name, it.InstanceType, it.PowerState, it.Zone, it.VPCID, it.ImageID, it.ImageName, it.ProviderCreationDate); e != nil {
				return fmt.Errorf("upsert kube node vm %q: %w", it.ProviderVMID, e)
			}
			keep = append(keep, it.ProviderVMID)
			res.Upserted++
		}
		tag, e := tx.Exec(ctx, del, accountID, keep)
		if e != nil {
			return fmt.Errorf("delete missing kube node vms: %w", e)
		}
		res.Deleted = int(tag.RowsAffected())

		rows, e := tx.Query(ctx, compute, accountID, grace.String())
		if e != nil {
			return fmt.Errorf("compute kube node vm status: %w", e)
		}
		defer rows.Close()
		for rows.Next() {
			var id, oldS, newS string
			if e := rows.Scan(&id, &oldS, &newS); e != nil {
				return fmt.Errorf("scan status: %w", e)
			}
			switch api.KubeNodeVMStatus(newS) {
			case api.KubeNodeVMStatusNode:
				res.Node++
			case api.KubeNodeVMStatusPending:
				res.Pending++
			case api.KubeNodeVMStatusOrphan:
				res.Orphan++
			case api.KubeNodeVMStatusUnknownCluster:
				res.UnknownCluster++
			}
			if oldS != newS {
				res.Transitions = append(res.Transitions, api.KubeNodeVMTransition{
					ProviderVMID: id, From: api.KubeNodeVMStatus(oldS), To: api.KubeNodeVMStatus(newS),
				})
			}
		}
		return rows.Err()
	})
	if err != nil {
		return api.KubeNodeVMReconcileResult{}, err
	}
	return res, nil
}

var kubeNodeVMSortSpec = sortSpec{
	columns: map[string]sortColumn{
		sortKeyName:                 {expr: "LOWER(k.name)", kind: sortText},
		sortKeyStatus:               {expr: "k.status", kind: sortText},
		sortKeyInstanceType:         {expr: "LOWER(k.instance_type)", kind: sortText},
		sortKeyPowerState:           {expr: "LOWER(k.power_state)", kind: sortText},
		sortKeyStatusSince:          {expr: "k.status_since", kind: sortTime},
		sortKeyFirstSeenAt:          {expr: "k.first_seen_at", kind: sortTime},
		sortKeyLastSeenAt:           {expr: "k.last_seen_at", kind: sortTime},
		sortKeyProviderCreationDate: {expr: "k.provider_creation_date", kind: sortTime, nullable: true},
	},
	defaultKey: sortKeyStatusSince,
	defaultDir: dirDesc,
}

func kubeNodeVMSortVal(v *api.KubeNodeVM, key string) *string {
	switch key {
	case sortKeyName:
		return sortValText(&v.Name)
	case sortKeyStatus:
		s := string(v.Status)
		return &s
	case sortKeyInstanceType:
		return sortValText(&v.InstanceType)
	case sortKeyPowerState:
		return sortValText(&v.PowerState)
	case sortKeyFirstSeenAt:
		return sortValTime(&v.FirstSeenAt)
	case sortKeyLastSeenAt:
		return sortValTime(&v.LastSeenAt)
	case sortKeyProviderCreationDate:
		return sortValTime(v.ProviderCreationDate)
	default:
		return sortValTime(&v.StatusSince)
	}
}

// ListKubeNodeVMs is the ADR-0042 list over kube_node_vms. Statuses empty =
// no status predicate (the HTTP/MCP layers apply the orphan+unknown default).
//
//nolint:gocyclo // cursor-paginated query builder with optional filters
func (p *PG) ListKubeNodeVMs(ctx context.Context, filter api.KubeNodeVMListFilter, page api.ListPage) ([]api.KubeNodeVM, string, error) {
	limit := clampLimit(page.Limit, 500)
	key, col, dir, err := kubeNodeVMSortSpec.resolve(page)
	if err != nil {
		return nil, "", err
	}
	sb := strings.Builder{}
	sb.WriteString("SELECT " + kubeNodeVMColumns + kubeNodeVMFrom)
	args := make([]any, 0, 8)
	conds := make([]string, 0, 8)
	if filter.CloudAccountID != nil {
		args = append(args, *filter.CloudAccountID)
		conds = append(conds, fmt.Sprintf("k.cloud_account_id = $%d", len(args)))
	}
	if filter.ClusterID != nil {
		args = append(args, *filter.ClusterID)
		conds = append(conds, fmt.Sprintf("k.cluster_id = $%d", len(args)))
	}
	if len(filter.Statuses) > 0 {
		ss := make([]string, len(filter.Statuses))
		for i := range filter.Statuses {
			ss[i] = string(filter.Statuses[i])
		}
		args = append(args, ss)
		conds = append(conds, fmt.Sprintf("k.status = ANY($%d)", len(args)))
	}
	if filter.ClusterHint != nil {
		args = append(args, *filter.ClusterHint)
		conds = append(conds, fmt.Sprintf("k.cluster_hint = $%d", len(args)))
	}
	if filter.InstanceType != nil {
		args = append(args, *filter.InstanceType)
		conds = append(conds, fmt.Sprintf("k.instance_type = $%d", len(args)))
	}
	if filter.PowerState != nil {
		args = append(args, *filter.PowerState)
		conds = append(conds, fmt.Sprintf("k.power_state = $%d", len(args)))
	}
	if filter.Name != nil {
		args = append(args, namePattern(*filter.Name))
		idx := len(args)
		conds = append(conds, fmt.Sprintf("(LOWER(k.name) LIKE $%d ESCAPE '\\' OR LOWER(k.provider_vm_id) LIKE $%d ESCAPE '\\')", idx, idx))
	}
	if page.Cursor != "" {
		val, cid, curErr := decodeListCursor(page.Cursor, key, dir)
		if curErr != nil {
			return nil, "", curErr
		}
		if curErr := keysetCond(col, "k.id", dir, val, cid, &conds, &args); curErr != nil {
			return nil, "", curErr
		}
	}
	if len(conds) > 0 {
		sb.WriteString(" WHERE " + strings.Join(conds, " AND "))
	}
	args = append(args, limit+1)
	fmt.Fprintf(&sb, " %s LIMIT $%d", orderBy(col, "k.id", dir), len(args))

	rows, err := p.pool.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, "", fmt.Errorf("query kube node vms: %w", err)
	}
	defer rows.Close()
	items := make([]api.KubeNodeVM, 0, limit)
	for rows.Next() {
		v, err := scanKubeNodeVM(rows)
		if err != nil {
			return nil, "", err
		}
		items = append(items, v)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate kube node vms: %w", err)
	}
	var next string
	if len(items) > limit {
		last := &items[limit-1]
		next = encodeListCursor(key, kubeNodeVMSortVal(last, key), last.ID, dir)
		items = items[:limit]
	}
	return items, next, nil
}

func scanKubeNodeVM(row pgx.Row) (api.KubeNodeVM, error) {
	var v api.KubeNodeVM
	if err := row.Scan(&v.ID, &v.CloudAccountID, &v.CloudAccountName, &v.ProviderVMID, &v.ClusterTag, &v.NodeNameTag,
		&v.ClusterHint, &v.Name, &v.InstanceType, &v.PowerState, &v.Zone, &v.VPCID, &v.ImageID, &v.ImageName,
		&v.ProviderCreationDate, &v.NodeID, &v.ClusterID, &v.ClusterName, &v.Status, &v.StatusSince, &v.FirstSeenAt, &v.LastSeenAt); err != nil {
		return api.KubeNodeVM{}, fmt.Errorf("scan kube node vm: %w", err)
	}
	return v, nil
}

// SummarizeKubeNodeVMs groups rows per (account, status, instance_type) in
// SQL and folds vCPU/RAM in Go via api.ParseInstanceType (unparsable
// types count 0). nil accountID = all accounts.
func (p *PG) SummarizeKubeNodeVMs(ctx context.Context, accountID *uuid.UUID) ([]api.KubeNodeVMSummaryRow, error) {
	q := `SELECT k.cloud_account_id, ca.name, k.status, k.instance_type, count(*)
	        FROM kube_node_vms k JOIN cloud_accounts ca ON ca.id = k.cloud_account_id`
	args := []any{}
	if accountID != nil {
		args = append(args, *accountID)
		q += " WHERE k.cloud_account_id = $1"
	}
	q += " GROUP BY k.cloud_account_id, ca.name, k.status, k.instance_type ORDER BY ca.name, k.status"
	rows, err := p.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("summarize kube node vms: %w", err)
	}
	defer rows.Close()
	type key struct {
		acct   uuid.UUID
		status api.KubeNodeVMStatus
	}
	agg := map[key]*api.KubeNodeVMSummaryRow{}
	order := []key{}
	for rows.Next() {
		var (
			acct   uuid.UUID
			name   string
			status string
			itype  string
			n      int
		)
		if err := rows.Scan(&acct, &name, &status, &itype, &n); err != nil {
			return nil, fmt.Errorf("scan summary: %w", err)
		}
		k := key{acct, api.KubeNodeVMStatus(status)}
		r, ok := agg[k]
		if !ok {
			r = &api.KubeNodeVMSummaryRow{CloudAccountID: acct, CloudAccountName: name, Status: k.status}
			agg[k] = r
			order = append(order, k)
		}
		r.Count += n
		if vcpu, mem, ok := api.ParseInstanceType(itype); ok {
			r.VCPU += vcpu * n
			r.MemoryGiB += mem * n
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate summary: %w", err)
	}
	out := make([]api.KubeNodeVMSummaryRow, 0, len(order))
	for _, k := range order {
		out = append(out, *agg[k])
	}
	return out, nil
}
