//nolint:noctx // httptest.NewRequest carries no request context in these unit tests
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/sthalbert/longue-vue/internal/auth"
)

func getKubeNodeVMs(t *testing.T, target string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, http.NoBody)
	req = req.WithContext(auth.WithCaller(req.Context(), readCaller()))
	rr := httptest.NewRecorder()
	HandleListKubeNodeVMs(newMemStore()).ServeHTTP(rr, req)
	return rr
}

func TestHandleListKubeNodeVMs_DefaultStatusFilter(t *testing.T) {
	resetKubeNodeVMFake()
	kubeNodeVMFake.items = []KubeNodeVM{
		{ID: uuid.New(), ProviderVMID: "i-1", Status: KubeNodeVMStatusOrphan},
	}
	rr := getKubeNodeVMs(t, "/v1/kube-node-vms")
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	got := kubeNodeVMFake.lastFilter.Statuses
	if len(got) != 2 || got[0] != KubeNodeVMStatusOrphan ||
		got[1] != KubeNodeVMStatusUnknownCluster {
		t.Fatalf("default statuses = %v; want [orphan unknown_cluster]", got)
	}
	var resp struct {
		Items      []KubeNodeVM `json:"items"`
		NextCursor *string      `json:"next_cursor"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil || len(resp.Items) != 1 {
		t.Fatalf("decode: err=%v items=%d", err, len(resp.Items))
	}
}

func TestHandleListKubeNodeVMs_ExplicitFilters(t *testing.T) {
	resetKubeNodeVMFake()
	acct := uuid.New()
	rr := getKubeNodeVMs(
		t,
		"/v1/kube-node-vms?status=node&status=pending&cloud_account_id="+acct.String()+"&cluster_hint=main-x&name=i-0",
	)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	f := kubeNodeVMFake.lastFilter
	if len(f.Statuses) != 2 || f.CloudAccountID == nil || *f.CloudAccountID != acct ||
		f.ClusterHint == nil ||
		*f.ClusterHint != "main-x" ||
		f.Name == nil ||
		*f.Name != "i-0" {
		t.Fatalf("filter %+v", f)
	}
}

func TestHandleListKubeNodeVMs_InvalidStatus(t *testing.T) {
	resetKubeNodeVMFake()
	rr := getKubeNodeVMs(t, "/v1/kube-node-vms?status=bogus")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d; want 400", rr.Code)
	}
}

func TestHandleListKubeNodeVMs_InvalidAccountID(t *testing.T) {
	resetKubeNodeVMFake()
	rr := getKubeNodeVMs(t, "/v1/kube-node-vms?cloud_account_id=nope")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d; want 400", rr.Code)
	}
}

func TestHandleKubeNodeVMSummary(t *testing.T) {
	resetKubeNodeVMFake()
	kubeNodeVMFake.summary = []KubeNodeVMSummaryRow{
		{
			CloudAccountName: "zex-preprod-std",
			Status:           KubeNodeVMStatusOrphan,
			Count:            13,
			VCPU:             104,
			MemoryGiB:        416,
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/kube-node-vms/summary", http.NoBody)
	req = req.WithContext(auth.WithCaller(req.Context(), readCaller()))
	rr := httptest.NewRecorder()
	HandleKubeNodeVMSummary(newMemStore()).ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Rows []KubeNodeVMSummaryRow `json:"rows"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil || len(resp.Rows) != 1 ||
		resp.Rows[0].Count != 13 {
		t.Fatalf("decode: err=%v rows=%+v", err, resp.Rows)
	}
}

func TestHandleListKubeNodeVMs_RequiresReadScope(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/kube-node-vms", http.NoBody)
	rr := httptest.NewRecorder()
	HandleListKubeNodeVMs(newMemStore()).ServeHTTP(rr, req) // no caller
	if rr.Code != http.StatusUnauthorized && rr.Code != http.StatusForbidden {
		t.Fatalf("status %d; want 401/403", rr.Code)
	}
}
