package store

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sthalbert/longue-vue/internal/api"
)

// seedKubeNodeVMFixture creates one cloud account, one cluster and one
// live node whose provider_id ends with "/i-live".
func seedKubeNodeVMFixture(t *testing.T, pg *PG) (accountID, clusterID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	acct, err := pg.UpsertCloudAccount(ctx, api.CloudAccountUpsert{Provider: "outscale", Name: "knv-" + uuid.NewString()[:8], Region: "eu-west-2"})
	if err != nil {
		t.Fatalf("UpsertCloudAccount: %v", err)
	}
	cluster, _, err := pg.EnsureCluster(ctx, api.ClusterCreate{Name: "knv-cluster-" + uuid.NewString()[:8]})
	if err != nil {
		t.Fatalf("EnsureCluster: %v", err)
	}
	pid := "aws:///eu-west-2a/i-live"
	if _, _, err := pg.UpsertNode(ctx, api.NodeCreate{ClusterId: *cluster.Id, Name: "node-live", ProviderId: &pid}); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	return acct.ID, *cluster.Id
}

func ago(d time.Duration) *time.Time { t := time.Now().UTC().Add(-d); return &t }

func statusOf(t *testing.T, pg *PG, accountID uuid.UUID, vmID string) api.KubeNodeVM {
	t.Helper()
	filter := api.KubeNodeVMListFilter{
		CloudAccountID: &accountID,
		Statuses:       []api.KubeNodeVMStatus{"node", "pending", "orphan", "unknown_cluster"},
	}
	items, _, err := pg.ListKubeNodeVMs(context.Background(), filter, api.ListPage{Limit: 100})
	if err != nil {
		t.Fatalf("ListKubeNodeVMs: %v", err)
	}
	for i := range items {
		if items[i].ProviderVMID == vmID {
			return items[i]
		}
	}
	t.Fatalf("vm %s not found", vmID)
	return api.KubeNodeVM{}
}

//nolint:gocyclo // integration test exercises every status-rule branch in one fixture
func TestReconcileKubeNodeVMs_StatusRules(t *testing.T) {
	pg := newTestPG(t)
	ctx := context.Background()
	accountID, clusterID := seedKubeNodeVMFixture(t, pg)
	grace := 24 * time.Hour

	items := []api.NodeImage{
		// matches the live node → node
		{
			ProviderVMID:         "i-live",
			Name:                 "main-x-md-worker-eu-west-2a-aaa",
			ClusterHint:          "main-x",
			ClusterTag:           "abc",
			InstanceType:         "tinav7.c8r32p1",
			ProviderCreationDate: ago(400 * time.Hour),
		},
		// same hint, old, no node → orphan
		{
			ProviderVMID:         "i-old",
			Name:                 "main-x-md-worker-eu-west-2b-bbb",
			ClusterHint:          "main-x",
			ClusterTag:           "main-x-oldformat",
			InstanceType:         "tinav7.c8r32p1",
			ProviderCreationDate: ago(400 * time.Hour),
		},
		// same hint, young → pending
		{
			ProviderVMID:         "i-new",
			Name:                 "main-x-md-worker-eu-west-2c-ccc",
			ClusterHint:          "main-x",
			ClusterTag:           "abc",
			ProviderCreationDate: ago(time.Hour),
		},
		// hint nobody matched, old → unknown_cluster
		{
			ProviderVMID:         "i-rome",
			Name:                 "rome01-md-worker-eu-west-2a-ddd",
			ClusterHint:          "rome01",
			ClusterTag:           "zzz",
			ProviderCreationDate: ago(400 * time.Hour),
		},
		// no hint, but exact cluster_tag shared with the node → orphan
		{ProviderVMID: "i-tagonly", Name: "weird-name", ClusterHint: "", ClusterTag: "abc", ProviderCreationDate: ago(400 * time.Hour)},
		// legacy collector row: no hint, no tag, no creation date → pending (grace from first_seen_at)
		{ProviderVMID: "i-legacy", ImageID: "ami-1", ImageName: "img"},
	}
	res, err := pg.ReconcileKubeNodeVMs(ctx, accountID, items, grace)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.Upserted != 6 || res.Deleted != 0 {
		t.Fatalf("upserted=%d deleted=%d; want 6,0", res.Upserted, res.Deleted)
	}
	want := map[string]api.KubeNodeVMStatus{
		"i-live": "node", "i-old": "orphan", "i-new": "pending",
		"i-rome": "unknown_cluster", "i-tagonly": "orphan", "i-legacy": "pending",
	}
	for id, st := range want {
		got := statusOf(t, pg, accountID, id)
		if got.Status != st {
			t.Errorf("%s: status %q; want %q", id, got.Status, st)
		}
		switch id {
		case "i-live":
			if got.NodeID == nil || got.ClusterID == nil || *got.ClusterID != clusterID {
				t.Errorf("i-live: node/cluster not linked: %+v", got)
			}
		case "i-old", "i-tagonly":
			if got.ClusterID == nil || *got.ClusterID != clusterID {
				t.Errorf("%s: orphan should carry the sibling cluster_id", id)
			}
		}
	}
	if res.Node != 1 || res.Orphan != 2 || res.Pending != 2 || res.UnknownCluster != 1 {
		t.Fatalf("counts %+v", res)
	}
}

func TestReconcileKubeNodeVMs_ZeroGraceClassifiesImmediately(t *testing.T) {
	pg := newTestPG(t)
	accountID, _ := seedKubeNodeVMFixture(t, pg)
	items := []api.NodeImage{
		{ProviderVMID: "i-live", ClusterHint: "main-x"},
		{ProviderVMID: "i-young", ClusterHint: "main-x", ProviderCreationDate: ago(time.Minute)},
	}
	if _, err := pg.ReconcileKubeNodeVMs(context.Background(), accountID, items, 0); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got := statusOf(t, pg, accountID, "i-young").Status; got != api.KubeNodeVMStatusOrphan {
		t.Fatalf("status %q; want orphan with zero grace", got)
	}
}

func TestReconcileKubeNodeVMs_DeletesMissingAndKeepsStatusSince(t *testing.T) {
	pg := newTestPG(t)
	ctx := context.Background()
	accountID, _ := seedKubeNodeVMFixture(t, pg)
	first := []api.NodeImage{
		{ProviderVMID: "i-live", ClusterHint: "main-x"},
		{ProviderVMID: "i-old", ClusterHint: "main-x", ProviderCreationDate: ago(400 * time.Hour)},
		{ProviderVMID: "i-gone", ClusterHint: "main-x", ProviderCreationDate: ago(400 * time.Hour)},
	}
	if _, err := pg.ReconcileKubeNodeVMs(ctx, accountID, first, 24*time.Hour); err != nil {
		t.Fatalf("first: %v", err)
	}
	since1 := statusOf(t, pg, accountID, "i-old").StatusSince

	second := first[:2] // i-gone disappeared from the cloud
	res, err := pg.ReconcileKubeNodeVMs(ctx, accountID, second, 24*time.Hour)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if res.Deleted != 1 {
		t.Fatalf("deleted=%d; want 1", res.Deleted)
	}
	if got := statusOf(t, pg, accountID, "i-old"); !got.StatusSince.Equal(since1) {
		t.Fatalf("status_since rewritten on unchanged status: %v → %v", since1, got.StatusSince)
	}
	if len(res.Transitions) != 0 {
		t.Fatalf("unexpected transitions: %+v", res.Transitions)
	}
}

func TestReconcileKubeNodeVMs_TransitionPendingToNode(t *testing.T) {
	pg := newTestPG(t)
	ctx := context.Background()
	accountID, clusterID := seedKubeNodeVMFixture(t, pg)
	items := []api.NodeImage{{ProviderVMID: "i-joining", ClusterHint: "main-x", ProviderCreationDate: ago(time.Minute)}}
	if _, err := pg.ReconcileKubeNodeVMs(ctx, accountID, items, 24*time.Hour); err != nil {
		t.Fatalf("first: %v", err)
	}
	// The VM joins the cluster: a node appears with its provider id.
	pid := "aws:///eu-west-2b/i-joining"
	if _, _, err := pg.UpsertNode(ctx, api.NodeCreate{ClusterId: clusterID, Name: "node-joining", ProviderId: &pid}); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}
	res, err := pg.ReconcileKubeNodeVMs(ctx, accountID, items, 24*time.Hour)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if len(res.Transitions) != 1 || res.Transitions[0].From != "pending" || res.Transitions[0].To != "node" {
		t.Fatalf("transitions %+v; want pending→node", res.Transitions)
	}
}

func TestReconcileKubeNodeVMs_SkipsInvalidProviderID(t *testing.T) {
	pg := newTestPG(t)
	accountID, _ := seedKubeNodeVMFixture(t, pg)
	items := []api.NodeImage{{ProviderVMID: "i-live"}, {ProviderVMID: "not a vm id; DROP TABLE"}}
	res, err := pg.ReconcileKubeNodeVMs(context.Background(), accountID, items, 0)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if res.Upserted != 1 {
		t.Fatalf("upserted=%d; want 1 (invalid id skipped)", res.Upserted)
	}
}
