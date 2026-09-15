package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sthalbert/longue-vue/internal/auth"
)

func TestHandleUpdateSettings_RejectsNegativeGrace(t *testing.T) {
	store := newMemStore()
	neg := -1
	b, _ := json.Marshal(SettingsPatch{KubeNodeVMGraceHours: &neg})
	req := httptest.NewRequest(http.MethodPatch, "/v1/admin/settings", bytes.NewReader(b)) //nolint:noctx // in-process handler test
	req = req.WithContext(auth.WithCaller(req.Context(), adminCaller()))
	rr := httptest.NewRecorder()
	HandleUpdateSettings(store).ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d; want 400", rr.Code)
	}
}
