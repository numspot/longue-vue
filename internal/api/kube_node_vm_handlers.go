// Kube-tagged VM read endpoints (ADR-0045 §4.1).
package api

import (
	"net/http"
	"net/url"

	"github.com/google/uuid"

	"github.com/sthalbert/longue-vue/internal/auth"
)

var kubeNodeVMDefaultStatuses = []KubeNodeVMStatus{KubeNodeVMStatusOrphan, KubeNodeVMStatusUnknownCluster}

// parseKubeNodeVMUUIDs fills the CloudAccountID/ClusterID filter fields
// from query params. Returns a problem detail ("" when valid).
func parseKubeNodeVMUUIDs(q url.Values, f *KubeNodeVMListFilter) string {
	if v := q.Get("cloud_account_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return "invalid cloud_account_id"
		}
		f.CloudAccountID = &id
	}
	if v := q.Get("cluster_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return "invalid cluster_id"
		}
		f.ClusterID = &id
	}
	return ""
}

// parseKubeNodeVMStatuses fills f.Statuses from the repeatable `status`
// query param, defaulting to orphan+unknown_cluster when absent. Returns
// a problem detail ("" when valid).
func parseKubeNodeVMStatuses(q url.Values, f *KubeNodeVMListFilter) string {
	statuses := q["status"]
	if len(statuses) == 0 {
		f.Statuses = kubeNodeVMDefaultStatuses
		return ""
	}
	for _, s := range statuses {
		switch KubeNodeVMStatus(s) {
		case KubeNodeVMStatusNode, KubeNodeVMStatusPending, KubeNodeVMStatusOrphan, KubeNodeVMStatusUnknownCluster:
			f.Statuses = append(f.Statuses, KubeNodeVMStatus(s))
		default:
			return "invalid status"
		}
	}
	return ""
}

// kubeNodeVMStringFilterFields maps each free-text query param to its
// destination field in KubeNodeVMListFilter.
var kubeNodeVMStringFilterFields = []struct {
	key string
	dst func(*KubeNodeVMListFilter) **string
}{
	{"cluster_hint", func(f *KubeNodeVMListFilter) **string { return &f.ClusterHint }},
	{"instance_type", func(f *KubeNodeVMListFilter) **string { return &f.InstanceType }},
	{"power_state", func(f *KubeNodeVMListFilter) **string { return &f.PowerState }},
	{"name", func(f *KubeNodeVMListFilter) **string { return &f.Name }},
}

// parseKubeNodeVMStrings fills the free-text filter fields (each capped at
// 200 chars). Returns a problem detail ("" when valid).
func parseKubeNodeVMStrings(q url.Values, f *KubeNodeVMListFilter) string {
	for _, p := range kubeNodeVMStringFilterFields {
		v := q.Get(p.key)
		if v == "" {
			continue
		}
		if len(v) > 200 {
			return p.key + " too long"
		}
		*p.dst(f) = &v
	}
	return ""
}

// parseKubeNodeVMFilter builds the list filter from query params. The
// second return value is a problem detail ("" when valid). status is
// repeatable; absent status = orphan + unknown_cluster.
func parseKubeNodeVMFilter(q url.Values) (filter KubeNodeVMListFilter, problem string) {
	if problem = parseKubeNodeVMUUIDs(q, &filter); problem != "" {
		return filter, problem
	}
	if problem = parseKubeNodeVMStatuses(q, &filter); problem != "" {
		return filter, problem
	}
	if problem = parseKubeNodeVMStrings(q, &filter); problem != "" {
		return filter, problem
	}
	return filter, ""
}

// HandleListKubeNodeVMs — read scope. GET /v1/kube-node-vms.
func HandleListKubeNodeVMs(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireScope(w, r, auth.ScopeRead) {
			return
		}
		filter, problem := parseKubeNodeVMFilter(r.URL.Query())
		if problem != "" {
			writeProblem(w, http.StatusBadRequest, "Bad Request", problem)
			return
		}
		items, next, err := store.ListKubeNodeVMs(r.Context(), filter, parseListPage(r))
		if err != nil {
			writeListError(w, "list kube node vms", err)
			return
		}
		if items == nil {
			items = []KubeNodeVM{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": next})
	}
}

// HandleKubeNodeVMSummary — read scope. GET /v1/kube-node-vms/summary
// (optional cloud_account_id).
func HandleKubeNodeVMSummary(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireScope(w, r, auth.ScopeRead) {
			return
		}
		var acct *uuid.UUID
		if v := r.URL.Query().Get("cloud_account_id"); v != "" {
			id, err := uuid.Parse(v)
			if err != nil {
				writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid cloud_account_id")
				return
			}
			acct = &id
		}
		rows, err := store.SummarizeKubeNodeVMs(r.Context(), acct)
		if err != nil {
			writeListError(w, "summarize kube node vms", err)
			return
		}
		if rows == nil {
			rows = []KubeNodeVMSummaryRow{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"rows": rows})
	}
}
