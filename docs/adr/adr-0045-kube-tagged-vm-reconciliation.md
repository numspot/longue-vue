---
title: "ADR-0045: Kube-tagged VM reconciliation — orphan node VMs"
status: "Accepted"
date: "2026-09-14"
authors: "Steve ALBERT"
tags: ["architecture", "decision", "cmdb", "vm-collector", "finops", "kubernetes"]
supersedes: ""
superseded_by: ""
---

# ADR-0045: Kube-tagged VM reconciliation — orphan node VMs

## Status

Proposed | **Accepted** | Rejected | Superseded | Deprecated

- **Date:** 2026-09-14
- **Supersedes:** none
- **Superseded by:** none

## Context

The VM collector drops every cloud VM tagged `OscK8sClusterID/*` or
`OscK8sNodeName` (ADR-0015: nodes are not platform VMs) and only forwards
them for OS-image backfill (ADR-0040). A tagged VM that matches no node was
counted in a global `nomatch` metric and forgotten. On 2026-09-14 a manual
sweep found 27 such VMs across four accounts — Cluster API workers left
behind by rollouts since 2025-10-27, ≈ 11 k€ HT/month — invisible to every
Longue-Vue inventory. The same sweep found kube-tagged VMs of clusters not
enrolled in the CMDB at all.

## Decision

Persist every kube-tagged VM the collector sees in `kube_node_vms`, fed by
the existing node-images ingest with an enriched payload, and compute a
status inside the ingest transaction:

- `node` — a live `nodes` row has the VM id as `provider_id` suffix;
- `pending` — no node, VM younger than `settings.kube_node_vm_grace_hours`
  (default 24, seeded from `LONGUE_VUE_KUBE_NODE_VM_GRACE_HOURS`);
- `orphan` — no node, grace elapsed, cluster known (a sibling VM sharing
  the name-derived `cluster_hint` or the exact cluster tag is a node);
- `unknown_cluster` — no node, grace elapsed, no sibling is a node.

Rows absent from a tick's payload are deleted. Exposure: `GET
/v1/kube-node-vms` (+ `/summary`), MCP `list_kube_node_vms`, a UI page,
gauges `longue_vue_kube_node_vms{cloud_account,status}` and
`longue_vue_kube_node_vms_vcpu`, with optional Helm alert rules.

Rejected: inventorying all VMs (reverses ADR-0015, bloats audit and VM
inventory, cannot tell orphan from un-enrolled cluster); an external
script (no UI, duplicated credentials, forgotten).

## Consequences

- ADR-0015 unchanged; no new scope, secret or cloud API call.
- Old collectors keep working (three-field payload → rows can only be
  `node` or `pending`); old servers ignore the new fields.
- A cluster whose collector dies turns its VMs into orphans at the next
  tick — desired (dead cluster, VMs still billed).
- Out of scope for now: detached volumes, unattached public IPs,
  backend-less load balancers, cost estimation in euros, purging stale
  `nodes` rows when a cluster disappears.
