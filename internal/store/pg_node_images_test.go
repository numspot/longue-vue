package store

import (
	"context"
	"testing"

	"github.com/sthalbert/longue-vue/internal/api"
)

//nolint:gocyclo // integration test exercises matched, updated, and no-op branches
func TestBackfillNodeImages(t *testing.T) {
	pg := newTestPG(t)
	ctx := context.Background()

	cluster, _, err := pg.EnsureCluster(ctx, api.ClusterCreate{Name: "bni-cluster"})
	if err != nil {
		t.Fatalf("EnsureCluster: %v", err)
	}
	providerID := "aws:///eu-west-2a/i-0abc123"
	if _, _, err := pg.UpsertNode(ctx, api.NodeCreate{
		ClusterId:  *cluster.Id,
		Name:       "node-1",
		ProviderId: &providerID,
	}); err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	// First backfill: matches by VmId substring, updates the row.
	matched, updated, err := pg.BackfillNodeImages(ctx, []api.NodeImage{
		{ProviderVMID: "i-0abc123", ImageID: "ami-1", ImageName: "master-k8s-1-32-2025.10.13"},
	})
	if err != nil {
		t.Fatalf("BackfillNodeImages: %v", err)
	}
	if matched != 1 || updated != 1 {
		t.Fatalf("first backfill matched=%d updated=%d; want 1,1", matched, updated)
	}

	// Idempotent: same values → matched 1, updated 0.
	matched, updated, err = pg.BackfillNodeImages(ctx, []api.NodeImage{
		{ProviderVMID: "i-0abc123", ImageID: "ami-1", ImageName: "master-k8s-1-32-2025.10.13"},
	})
	if err != nil {
		t.Fatalf("BackfillNodeImages idempotent: %v", err)
	}
	if matched != 1 || updated != 0 {
		t.Fatalf("idempotent backfill matched=%d updated=%d; want 1,0", matched, updated)
	}

	// No matching node → no-op.
	matched, updated, err = pg.BackfillNodeImages(ctx, []api.NodeImage{
		{ProviderVMID: "i-nomatch", ImageID: "ami-2", ImageName: "other"},
	})
	if err != nil {
		t.Fatalf("BackfillNodeImages nomatch: %v", err)
	}
	if matched != 0 || updated != 0 {
		t.Fatalf("nomatch backfill matched=%d updated=%d; want 0,0", matched, updated)
	}
}

// TestBackfillNodeImages_EmptyImageDoesNotClear pins the ADR-0040 regression
// fix (finding 1, 2026-09-14 review): a kube-tagged VM reported without a
// resolved image (ADR-0045 §2 — the new collector now sends imageless kube
// VMs so reconciliation can see them; also the profile of a transient
// ReadImages failure or a deregistered AMI) must never null out a
// previously backfilled node image.
//
//nolint:gocyclo // integration test exercises seed, first backfill, empty-image backfill, and verification branches
func TestBackfillNodeImages_EmptyImageDoesNotClear(t *testing.T) {
	pg := newTestPG(t)
	ctx := context.Background()

	cluster, _, err := pg.EnsureCluster(ctx, api.ClusterCreate{Name: "bni-empty-cluster"})
	if err != nil {
		t.Fatalf("EnsureCluster: %v", err)
	}
	providerID := "aws:///eu-west-2a/i-0keepme"
	created, _, err := pg.UpsertNode(ctx, api.NodeCreate{
		ClusterId:  *cluster.Id,
		Name:       "node-keepme",
		ProviderId: &providerID,
	})
	if err != nil {
		t.Fatalf("UpsertNode: %v", err)
	}

	// Backfill a real image first.
	matched, updated, err := pg.BackfillNodeImages(ctx, []api.NodeImage{
		{ProviderVMID: "i-0keepme", ImageID: "ami-1", ImageName: "name"},
	})
	if err != nil {
		t.Fatalf("BackfillNodeImages: %v", err)
	}
	if matched != 1 || updated != 1 {
		t.Fatalf("first backfill matched=%d updated=%d; want 1,1", matched, updated)
	}

	// Same provider id, empty image fields (e.g. a kube-tagged VM with no
	// resolved image, or a transient ReadImages failure): must not clear
	// the previously backfilled values. The node is still matched.
	matched, updated, err = pg.BackfillNodeImages(ctx, []api.NodeImage{
		{ProviderVMID: "i-0keepme", ImageID: "", ImageName: ""},
	})
	if err != nil {
		t.Fatalf("BackfillNodeImages empty: %v", err)
	}
	if matched != 1 || updated != 0 {
		t.Fatalf("empty-image backfill matched=%d updated=%d; want 1,0 (no clear)", matched, updated)
	}

	var imageID, imageName *string
	if err := pg.pool.QueryRow(ctx,
		`SELECT image_id, image_name FROM nodes WHERE id = $1`, *created.Id,
	).Scan(&imageID, &imageName); err != nil {
		t.Fatalf("query node image columns: %v", err)
	}
	if imageID == nil || *imageID != "ami-1" {
		t.Errorf("image_id = %v; want ami-1 (must not be cleared)", imageID)
	}
	if imageName == nil || *imageName != "name" {
		t.Errorf("image_name = %v; want name (must not be cleared)", imageName)
	}
}
