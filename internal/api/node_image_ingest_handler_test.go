//nolint:noctx // httptest.NewRequest carries no request context in these unit tests
package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sthalbert/longue-vue/internal/auth"
)

func postNodeImages(t *testing.T, store Store, caller *auth.Caller, accID uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost,
		"/v1/ingest/cloud-accounts/"+accID.String()+"/node-images", bytes.NewReader(b))
	req.SetPathValue("id", accID.String())
	if caller != nil {
		req = req.WithContext(auth.WithCaller(req.Context(), caller))
	}
	rr := httptest.NewRecorder()
	HandleBackfillNodeImages(store).ServeHTTP(rr, req)
	return rr
}

func TestHandleBackfillNodeImages_OK(t *testing.T) {
	resetOSImageFake()
	osImageFake.matched = 2
	osImageFake.updated = 1
	accID := uuid.New()
	store := newMemStore()

	rr := postNodeImages(t, store, collectorCaller(&accID), accID, backfillNodeImagesRequest{
		Images: []NodeImage{{ProviderVMID: "i-1", ImageID: "ami-1", ImageName: "img-node"}},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Matched int `json:"matched"`
		Updated int `json:"updated"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Matched != 2 || resp.Updated != 1 {
		t.Fatalf("matched=%d updated=%d; want 2,1", resp.Matched, resp.Updated)
	}
	if len(osImageFake.backfilled) != 1 || osImageFake.backfilled[0].ProviderVMID != "i-1" {
		t.Fatalf("store not called as expected: %+v", osImageFake.backfilled)
	}
}

func TestHandleBackfillNodeImages_WrongScope(t *testing.T) {
	resetOSImageFake()
	accID := uuid.New()
	rr := postNodeImages(t, newMemStore(), readCaller(), accID, backfillNodeImagesRequest{})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status %d; want 403", rr.Code)
	}
}

func TestHandleBackfillNodeImages_WrongAccountBinding(t *testing.T) {
	resetOSImageFake()
	boundID := uuid.New()
	otherID := uuid.New()
	rr := postNodeImages(t, newMemStore(), collectorCaller(&boundID), otherID, backfillNodeImagesRequest{})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status %d; want 403", rr.Code)
	}
}

func TestHandleBackfillNodeImages_ReconcilesKubeNodeVMs(t *testing.T) {
	resetOSImageFake()
	resetKubeNodeVMFake()
	kubeNodeVMFake.result = KubeNodeVMReconcileResult{
		Upserted: 2, Orphan: 1, Node: 1,
		Transitions: []KubeNodeVMTransition{{ProviderVMID: "i-2", From: "pending", To: "orphan"}},
	}
	accID := uuid.New()
	store := newMemStore()
	store.settings.KubeNodeVMGraceHours = 6

	rr := postNodeImages(t, store, collectorCaller(&accID), accID, backfillNodeImagesRequest{
		Images: []NodeImage{
			{ProviderVMID: "i-1", ImageID: "ami-1", ImageName: "img", ClusterHint: "main-x"},
			{ProviderVMID: "i-2", ClusterHint: "main-x"},
		},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	if kubeNodeVMFake.lastAccount != accID || len(kubeNodeVMFake.reconciled) != 2 {
		t.Fatalf("reconcile not called as expected: acct=%s n=%d", kubeNodeVMFake.lastAccount, len(kubeNodeVMFake.reconciled))
	}
	if kubeNodeVMFake.lastGrace != 6*time.Hour {
		t.Fatalf("grace %v; want 6h from settings", kubeNodeVMFake.lastGrace)
	}
	var resp struct {
		KubeNodeVMs struct {
			Upserted int `json:"upserted"`
			Orphan   int `json:"orphan"`
		} `json:"kube_node_vms"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.KubeNodeVMs.Upserted != 2 || resp.KubeNodeVMs.Orphan != 1 {
		t.Fatalf("unexpected kube_node_vms in response: %+v", resp.KubeNodeVMs)
	}
}

// Legacy three-field payload: still accepted, still reconciled (rows
// without hint/date can only be node or pending, never a false orphan).
func TestHandleBackfillNodeImages_LegacyPayloadStillReconciles(t *testing.T) {
	resetOSImageFake()
	resetKubeNodeVMFake()
	accID := uuid.New()
	store := newMemStore()
	store.settings.KubeNodeVMGraceHours = 24
	rr := postNodeImages(t, store, collectorCaller(&accID), accID,
		map[string]any{"images": []map[string]string{{"provider_vm_id": "i-1", "image_id": "ami-1", "image_name": "img"}}})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rr.Code, rr.Body.String())
	}
	if len(kubeNodeVMFake.reconciled) != 1 || kubeNodeVMFake.reconciled[0].ClusterHint != "" {
		t.Fatalf("legacy payload not reconciled: %+v", kubeNodeVMFake.reconciled)
	}
	if kubeNodeVMFake.lastGrace != 24*time.Hour {
		t.Fatalf("grace %v; want default 24h", kubeNodeVMFake.lastGrace)
	}
}

// A reconcile failure must not fail the OS-image backfill response
// (best-effort, logged), so mixed-version rollouts never break the tick.
func TestHandleBackfillNodeImages_ReconcileErrorIsNonFatal(t *testing.T) {
	resetOSImageFake()
	resetKubeNodeVMFake()
	kubeNodeVMFake.err = errors.New("boom")
	accID := uuid.New()
	rr := postNodeImages(t, newMemStore(), collectorCaller(&accID), accID, backfillNodeImagesRequest{
		Images: []NodeImage{{ProviderVMID: "i-1"}},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d; want 200 despite reconcile error", rr.Code)
	}
}
