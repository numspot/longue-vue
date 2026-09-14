package api

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// kubeNodeVMFake records ReconcileKubeNodeVMs input and serves fixtures
// for the list/summary handlers (same pattern as osImageFake).
var kubeNodeVMFake = struct {
	mu          sync.Mutex
	reconciled  []NodeImage
	lastAccount uuid.UUID
	lastGrace   time.Duration
	result      KubeNodeVMReconcileResult
	items       []KubeNodeVM
	lastFilter  KubeNodeVMListFilter
	summary     []KubeNodeVMSummaryRow
	err         error
}{}

func resetKubeNodeVMFake() {
	kubeNodeVMFake.mu.Lock()
	defer kubeNodeVMFake.mu.Unlock()
	kubeNodeVMFake.reconciled = nil
	kubeNodeVMFake.lastAccount = uuid.Nil
	kubeNodeVMFake.lastGrace = 0
	kubeNodeVMFake.result = KubeNodeVMReconcileResult{}
	kubeNodeVMFake.items = nil
	kubeNodeVMFake.lastFilter = KubeNodeVMListFilter{}
	kubeNodeVMFake.summary = nil
	kubeNodeVMFake.err = nil
}

func (m *memStore) ReconcileKubeNodeVMs(
	_ context.Context, accountID uuid.UUID, items []NodeImage, grace time.Duration,
) (KubeNodeVMReconcileResult, error) {
	kubeNodeVMFake.mu.Lock()
	defer kubeNodeVMFake.mu.Unlock()
	kubeNodeVMFake.reconciled = append(kubeNodeVMFake.reconciled, items...)
	kubeNodeVMFake.lastAccount = accountID
	kubeNodeVMFake.lastGrace = grace
	return kubeNodeVMFake.result, kubeNodeVMFake.err
}

func (m *memStore) ListKubeNodeVMs(_ context.Context, filter KubeNodeVMListFilter, _ ListPage) ([]KubeNodeVM, string, error) {
	kubeNodeVMFake.mu.Lock()
	defer kubeNodeVMFake.mu.Unlock()
	kubeNodeVMFake.lastFilter = filter
	return kubeNodeVMFake.items, "", kubeNodeVMFake.err
}

func (m *memStore) SummarizeKubeNodeVMs(_ context.Context, _ *uuid.UUID) ([]KubeNodeVMSummaryRow, error) {
	kubeNodeVMFake.mu.Lock()
	defer kubeNodeVMFake.mu.Unlock()
	return kubeNodeVMFake.summary, kubeNodeVMFake.err
}
