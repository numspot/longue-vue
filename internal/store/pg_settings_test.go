package store

import (
	"context"
	"testing"

	"github.com/sthalbert/longue-vue/internal/api"
)

func TestSettings_KubeNodeVMGraceHours(t *testing.T) {
	pg := newTestPG(t)
	// The settings table is a shared singleton row that newTestPG's
	// TRUNCATE cleanup does not touch (see pg_settings_stale_test.go).
	// Registering this cleanup after newTestPG's own means it runs
	// first (LIFO), while the pool is still open, restoring the
	// default for subsequent test runs against the same database.
	t.Cleanup(func() {
		twentyFour := 24
		if _, err := pg.UpdateSettings(context.Background(), api.SettingsPatch{KubeNodeVMGraceHours: &twentyFour}); err != nil {
			t.Errorf("cleanup: restore kube_node_vm_grace_hours: %v", err)
		}
	})
	ctx := context.Background()

	s, err := pg.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if s.KubeNodeVMGraceHours != 24 {
		t.Fatalf("default kube_node_vm_grace_hours = %d; want 24", s.KubeNodeVMGraceHours)
	}

	h := 6
	s, err = pg.UpdateSettings(ctx, api.SettingsPatch{KubeNodeVMGraceHours: &h})
	if err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}
	if s.KubeNodeVMGraceHours != 6 {
		t.Fatalf("patched kube_node_vm_grace_hours = %d; want 6", s.KubeNodeVMGraceHours)
	}

	// Nil field leaves the value alone.
	s, err = pg.UpdateSettings(ctx, api.SettingsPatch{})
	if err != nil {
		t.Fatalf("UpdateSettings no-op: %v", err)
	}
	if s.KubeNodeVMGraceHours != 6 {
		t.Fatalf("no-op patch changed value to %d", s.KubeNodeVMGraceHours)
	}
}
