package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sthalbert/longue-vue/internal/api"
)

func newKubeNodeVM(acct uuid.UUID, id string, st api.KubeNodeVMStatus) api.KubeNodeVM {
	now := time.Now().UTC()
	return api.KubeNodeVM{
		ID: uuid.New(), CloudAccountID: acct, CloudAccountName: "acct", ProviderVMID: id, Name: "main-x-md-worker-" + id,
		ClusterHint: "main-x", InstanceType: "tinav7.c8r32p1", Status: st, StatusSince: now, FirstSeenAt: now, LastSeenAt: now,
	}
}

func TestHandleListKubeNodeVMs_DefaultsToOrphanAndUnknown(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	acct := uuid.New()
	store.kubeNodeVMs = []api.KubeNodeVM{
		newKubeNodeVM(acct, "i-node", api.KubeNodeVMStatusNode),
		newKubeNodeVM(acct, "i-orph", api.KubeNodeVMStatusOrphan),
		newKubeNodeVM(acct, "i-unk", api.KubeNodeVMStatusUnknownCluster),
	}
	s := newServer(t, store)
	r, err := s.handleListKubeNodeVMs(context.Background(), makeRequest("", nil))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got []api.KubeNodeVM
	if err := json.Unmarshal([]byte(resultText(t, r)), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d; want 2 (node excluded by default)", len(got))
	}
	if len(store.lastKubeNodeVMFilter.Statuses) != 2 {
		t.Fatalf("filter statuses %v", store.lastKubeNodeVMFilter.Statuses)
	}
}

func TestHandleListKubeNodeVMs_InvalidStatus(t *testing.T) {
	t.Parallel()
	s := newServer(t, newFakeStore())
	r, err := s.handleListKubeNodeVMs(context.Background(), makeRequest("", map[string]any{"status": "bogus"}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !r.IsError {
		t.Fatal("expected tool error for invalid status")
	}
}

func TestHandleGetCloudAccount_IncludesKubeNodeVMSummary(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	acct := uuid.New()
	store.accounts = []api.CloudAccount{{ID: acct, Provider: "outscale", Name: "acct", Region: "eu-west-2", Status: "active"}}
	store.kubeNodeVMs = []api.KubeNodeVM{
		newKubeNodeVM(acct, "i-orph", api.KubeNodeVMStatusOrphan),
		newKubeNodeVM(acct, "i-orph2", api.KubeNodeVMStatusOrphan),
		newKubeNodeVM(acct, "i-node", api.KubeNodeVMStatusNode),
	}
	s := newServer(t, store)
	r, err := s.handleGetCloudAccount(context.Background(), makeRequest("", map[string]any{"id": acct.String()}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var got struct {
		KubeNodeVMs map[string]int `json:"kube_node_vms"`
	}
	if err := json.Unmarshal([]byte(resultText(t, r)), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.KubeNodeVMs["orphan"] != 2 || got.KubeNodeVMs["node"] != 1 || got.KubeNodeVMs["pending"] != 0 {
		t.Fatalf("summary %v", got.KubeNodeVMs)
	}
}
