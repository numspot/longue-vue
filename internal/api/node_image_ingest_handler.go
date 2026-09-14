package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/sthalbert/longue-vue/internal/auth"
	"github.com/sthalbert/longue-vue/internal/metrics"
)

// backfillNodeImagesRequest is the body for
// POST /v1/ingest/cloud-accounts/{id}/node-images.
type backfillNodeImagesRequest struct {
	Images []NodeImage `json:"images"`
}

// HandleBackfillNodeImages — vm-collector scope, bound to the cloud account.
// Backfills nodes.image_id/image_name from the reported node-VM mappings
// (ADR-0040), then reconciles the kube-tagged VMs of the account against
// cluster nodes to surface orphans and unknown-cluster VMs (ADR-0045).
// Vendor-neutral CMDB inventory; no outbound calls.
//
// POST /v1/ingest/cloud-accounts/{id}/node-images
// Response: 200 {"matched":N,"updated":M,"kube_node_vms":{"upserted":N,
// "deleted":M,"node":K,"pending":P,"orphan":O,"unknown_cluster":U}}.
func HandleBackfillNodeImages(store Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireScope(w, r, auth.ScopeVMCollector) {
			return
		}
		caller := auth.CallerFromContext(r.Context())
		id, ok := pathUUID(w, r, "id")
		if !ok {
			return
		}
		if err := caller.EnforceCloudAccountBinding(id); err != nil {
			writeProblem(w, http.StatusForbidden, "Forbidden", "token not bound to this cloud account")
			return
		}
		var body backfillNodeImagesRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeProblem(w, http.StatusBadRequest, "Bad Request", "invalid JSON body")
			return
		}
		matched, updated, err := store.BackfillNodeImages(r.Context(), body.Images)
		if err != nil {
			slog.Error("backfill node images", slog.Any("cloud_account_id", id), slog.Any("error", err))
			writeProblem(w, http.StatusInternalServerError, "Internal Server Error", "")
			return
		}
		if matched > 0 {
			metrics.ObserveNodeImageBackfill("matched")
		} else {
			metrics.ObserveNodeImageBackfill("nomatch")
		}

		// ADR-0045: persist every kube-tagged VM and compute its status.
		// Best-effort: a failure is logged and the backfill response still
		// succeeds, so an old server/new collector mix never breaks the tick.
		grace := kubeNodeVMGrace(r.Context(), store)
		rec, rerr := store.ReconcileKubeNodeVMs(r.Context(), id, body.Images, grace)
		if rerr != nil {
			slog.Error("reconcile kube node vms", slog.Any("cloud_account_id", id), slog.Any("error", rerr))
		}
		for _, tr := range rec.Transitions {
			slog.Info("kube node vm status changed",
				slog.Any("cloud_account_id", id), slog.String("provider_vm_id", tr.ProviderVMID),
				slog.String("from", string(tr.From)), slog.String("to", string(tr.To)))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"matched": matched, "updated": updated,
			"kube_node_vms": map[string]int{
				"upserted": rec.Upserted, "deleted": rec.Deleted, "node": rec.Node,
				"pending": rec.Pending, "orphan": rec.Orphan, "unknown_cluster": rec.UnknownCluster,
			},
		})
	}
}

// kubeNodeVMGrace reads settings.kube_node_vm_grace_hours; on a settings
// read error it falls back to the documented default (24h) rather than
// classifying immediately.
func kubeNodeVMGrace(ctx context.Context, store Store) time.Duration {
	st, err := store.GetSettings(ctx)
	if err != nil {
		slog.Warn("kube node vms: settings unavailable, using 24h grace", slog.Any("error", err))
		return 24 * time.Hour
	}
	return time.Duration(st.KubeNodeVMGraceHours) * time.Hour
}
