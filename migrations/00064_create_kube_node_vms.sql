-- +goose Up
-- Kube-tagged VM reconciliation (ADR-0045). One row per cloud VM that
-- carries an OscK8sClusterID/* or OscK8sNodeName tag, per cloud account,
-- fed by POST /v1/ingest/cloud-accounts/{id}/node-images. Status is
-- computed inside the ingest transaction:
--   node            - a live nodes row has this provider_vm_id as provider_id suffix
--   pending         - no node yet, VM younger than the grace period
--   orphan          - no node, grace elapsed, cluster known (a sibling row is a node)
--   unknown_cluster - no node, grace elapsed, no sibling row is a node
-- Live nodes stay out of virtual_machines (ADR-0015 unchanged).
CREATE TABLE kube_node_vms (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cloud_account_id       UUID NOT NULL REFERENCES cloud_accounts(id) ON DELETE CASCADE,
    provider_vm_id         TEXT NOT NULL,
    cluster_tag            TEXT NOT NULL DEFAULT '',
    node_name_tag          TEXT NOT NULL DEFAULT '',
    cluster_hint           TEXT NOT NULL DEFAULT '',
    name                   TEXT NOT NULL DEFAULT '',
    instance_type          TEXT NOT NULL DEFAULT '',
    power_state            TEXT NOT NULL DEFAULT '',
    zone                   TEXT NOT NULL DEFAULT '',
    vpc_id                 TEXT NOT NULL DEFAULT '',
    image_id               TEXT NOT NULL DEFAULT '',
    image_name             TEXT NOT NULL DEFAULT '',
    provider_creation_date TIMESTAMPTZ,
    node_id                UUID REFERENCES nodes(id) ON DELETE SET NULL,
    cluster_id             UUID REFERENCES clusters(id) ON DELETE SET NULL,
    status                 TEXT NOT NULL DEFAULT 'pending'
                           CHECK (status IN ('node', 'pending', 'orphan', 'unknown_cluster')),
    status_since           TIMESTAMPTZ NOT NULL DEFAULT now(),
    first_seen_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (cloud_account_id, provider_vm_id)
);
CREATE INDEX kube_node_vms_account_status_idx ON kube_node_vms (cloud_account_id, status);
CREATE INDEX kube_node_vms_cluster_id_idx     ON kube_node_vms (cluster_id);
CREATE INDEX kube_node_vms_cluster_hint_idx   ON kube_node_vms (cluster_hint);

-- Grace period (hours) before a kube-tagged VM without a node is classified
-- orphan / unknown_cluster. 0 = classify immediately. Seeded from
-- LONGUE_VUE_KUBE_NODE_VM_GRACE_HOURS, hot-editable via PATCH /v1/admin/settings.
ALTER TABLE settings
  ADD COLUMN IF NOT EXISTS kube_node_vm_grace_hours INTEGER NOT NULL DEFAULT 24;

-- +goose Down
ALTER TABLE settings DROP COLUMN IF EXISTS kube_node_vm_grace_hours;
DROP TABLE IF EXISTS kube_node_vms;
