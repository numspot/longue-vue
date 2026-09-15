package metricsrefresh

import (
	"context"
	"testing"

	"github.com/sthalbert/longue-vue/internal/metrics"
)

type knvFakeStore struct {
	hbFakeStore
	rows []metrics.KubeNodeVMCount
}

func (f *knvFakeStore) KubeNodeVMCounts(context.Context) ([]metrics.KubeNodeVMCount, error) {
	return f.rows, nil
}

func gaugeValue(t *testing.T, name, account, status string) (float64, bool) {
	t.Helper()
	for _, m := range gatherGauge(t, name) {
		var acct, st string
		for _, l := range m.GetLabel() {
			switch l.GetName() {
			case "cloud_account":
				acct = l.GetValue()
			case "status":
				st = l.GetValue()
			}
		}
		if acct == account && st == status {
			return m.GetGauge().GetValue(), true
		}
	}
	return 0, false
}

func TestRefreshKubeNodeVMs_SetsAndResetsSeries(t *testing.T) {
	store := &knvFakeStore{rows: []metrics.KubeNodeVMCount{
		{CloudAccount: "zex-preprod-std", Status: "orphan", Count: 13, VCPU: 104},
		{CloudAccount: "zex-preprod-std", Status: "node", Count: 28, VCPU: 260},
	}}
	r := New(store, 0)
	r.refreshKubeNodeVMs(context.Background())

	if v, ok := gaugeValue(t, "longue_vue_kube_node_vms", "zex-preprod-std", "orphan"); !ok || v != 13 {
		t.Fatalf("orphan gauge = %v ok=%v; want 13", v, ok)
	}
	if v, ok := gaugeValue(t, "longue_vue_kube_node_vms_vcpu", "zex-preprod-std", "orphan"); !ok || v != 104 {
		t.Fatalf("orphan vcpu gauge = %v ok=%v; want 104", v, ok)
	}

	// Account disappears: its series must be reset, not left stale.
	store.rows = nil
	r.refreshKubeNodeVMs(context.Background())
	if _, ok := gaugeValue(t, "longue_vue_kube_node_vms", "zex-preprod-std", "orphan"); ok {
		t.Fatal("stale series survived a refresh with no rows")
	}
}
