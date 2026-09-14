package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/sthalbert/longue-vue/internal/api"
)

// ReconcileKubeNodeVMs, ListKubeNodeVMs and SummarizeKubeNodeVMs are
// temporary stubs (ADR-0045 Task 2) so *PG satisfies api.Store while the
// kube_node_vms table's real read/write paths are implemented in Task 3
// (reconcile) and Task 6 (list/summary).
var errKubeNodeVMsNotImplemented = errors.New("kube node vms: not implemented")

// ReconcileKubeNodeVMs is a temporary stub; see Task 3.
func (p *PG) ReconcileKubeNodeVMs(_ context.Context, _ uuid.UUID, _ []api.NodeImage, _ time.Duration) (api.KubeNodeVMReconcileResult, error) {
	return api.KubeNodeVMReconcileResult{}, errKubeNodeVMsNotImplemented
}

// ListKubeNodeVMs is a temporary stub; see Task 6.
func (p *PG) ListKubeNodeVMs(_ context.Context, _ api.KubeNodeVMListFilter, _ api.ListPage) ([]api.KubeNodeVM, string, error) {
	return nil, "", errKubeNodeVMsNotImplemented
}

// SummarizeKubeNodeVMs is a temporary stub; see Task 6.
func (p *PG) SummarizeKubeNodeVMs(_ context.Context, _ *uuid.UUID) ([]api.KubeNodeVMSummaryRow, error) {
	return nil, errKubeNodeVMsNotImplemented
}
