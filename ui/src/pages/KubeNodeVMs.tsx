import { useState } from 'react';
import { Link, useSearchParams } from 'react-router';
import * as api from '../api';
import { useResource, useDebouncedValue } from '../hooks';
import { AsyncView, Dash } from '../components';

// KubeNodeVMs — ADR-0045 reconciliation between cloud VMs tagged as
// Kubernetes nodes and the CMDB's own node inventory. The default view
// (no `status` query param sent) only surfaces the actionable statuses:
// orphan (backs no live node, billed for nothing) and unknown_cluster
// (can't even be matched to a known cluster).

type StatusFilter = api.KubeNodeVMStatus | 'default' | 'all';

const ALL_STATUSES: api.KubeNodeVMStatus[] = ['node', 'pending', 'orphan', 'unknown_cluster'];
const DEFAULT_STATUSES: api.KubeNodeVMStatus[] = ['orphan', 'unknown_cluster'];

export function statusPill(s: api.KubeNodeVMStatus): { cls: string; label: string } {
  switch (s) {
    case 'orphan':
      return { cls: 'pill status-warn', label: 'Orphan' };
    case 'unknown_cluster':
      return { cls: 'pill status-bad', label: 'Unknown cluster' };
    case 'pending':
      return { cls: 'pill', label: 'Pending' };
    default:
      return { cls: 'pill status-ok', label: 'Node' };
  }
}

function since(iso: string): string {
  const days = Math.floor((Date.now() - new Date(iso).getTime()) / 86_400_000);
  return days <= 0 ? 'today' : `${days} d`;
}

export default function KubeNodeVMs() {
  const [searchParams] = useSearchParams();
  const cloudAccountId = searchParams.get('cloud_account_id') || undefined;
  const clusterId = searchParams.get('cluster_id') || undefined;

  const [nameInput, setNameInput] = useState('');
  const [status, setStatus] = useState<StatusFilter>('default');
  const debouncedName = useDebouncedValue(nameInput.trim(), 300);
  const [extra, setExtra] = useState<api.KubeNodeVM[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);

  const statusParam =
    status === 'default' ? undefined : status === 'all' ? ALL_STATUSES : [status];

  const list = useResource(async () => {
    setExtra([]);
    const resp = await api.listKubeNodeVMs({
      cloud_account_id: cloudAccountId,
      cluster_id: clusterId,
      name: debouncedName || undefined,
      status: statusParam,
    });
    setCursor(resp.next_cursor ?? null);
    return resp;
  }, [cloudAccountId, clusterId, debouncedName, status]);

  const summary = useResource(() => api.summarizeKubeNodeVMs(cloudAccountId), [cloudAccountId]);

  const loadMore = async () => {
    if (!cursor) return;
    const resp = await api.listKubeNodeVMs({
      cloud_account_id: cloudAccountId,
      cluster_id: clusterId,
      name: debouncedName || undefined,
      status: statusParam,
      cursor,
    });
    setExtra((prev) => [...prev, ...resp.items]);
    setCursor(resp.next_cursor ?? null);
  };

  return (
    <div className="page">
      <div className="page-header">
        <h1>Node VMs</h1>
      </div>
      <p className="muted" style={{ marginBottom: '1rem' }}>
        Cloud VMs tagged as Kubernetes nodes, reconciled against the CMDB's node inventory
        (ADR-0045). Orphans back no live node and are billed for nothing.
      </p>
      <AsyncView state={summary}>
        {(s) => {
          const byStatus = (st: api.KubeNodeVMStatus) => s.rows.filter((r) => r.status === st);
          return (
            <div className="eol-summary" style={{ marginBottom: '1rem' }}>
              {DEFAULT_STATUSES.concat(['pending', 'node']).map((st) => {
                const rows = byStatus(st);
                const count = rows.reduce((n, r) => n + r.count, 0);
                const vcpu = rows.reduce((n, r) => n + r.vcpu, 0);
                const p = statusPill(st);
                return (
                  <div
                    key={st}
                    className={`eol-summary-card${status === st ? ' eol-active' : ''}`}
                    onClick={() => setStatus(status === st ? 'default' : st)}
                    role="button"
                    tabIndex={0}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' || e.key === ' ') setStatus(status === st ? 'default' : st);
                    }}
                  >
                    <span className="eol-summary-count">{count}</span>
                    <span className="eol-summary-label">{p.label} VMs · {vcpu} vCPU</span>
                  </div>
                );
              })}
            </div>
          );
        }}
      </AsyncView>
      <div style={{ display: 'flex', gap: '0.5rem', marginBottom: '0.75rem', alignItems: 'center' }}>
        <input
          type="search"
          placeholder="Name or VM id…"
          value={nameInput}
          onChange={(e) => setNameInput(e.target.value)}
        />
        <select value={status} onChange={(e) => setStatus(e.target.value as StatusFilter)}>
          <option value="default">Orphan + unknown cluster</option>
          <option value="all">All statuses</option>
          {ALL_STATUSES.map((st) => (
            <option key={st} value={st}>
              {statusPill(st).label} only
            </option>
          ))}
        </select>
      </div>
      <AsyncView state={list}>
        {(resp) => {
          const items = [...resp.items, ...extra];
          return (
            <>
              <div className="table-wrap">
                <table className="entities">
                  <thead>
                    <tr>
                      <th>VM</th>
                      <th>Name</th>
                      <th>Status</th>
                      <th>Since</th>
                      <th>Account</th>
                      <th>Cluster</th>
                      <th>Type</th>
                      <th>Power</th>
                      <th>Created</th>
                    </tr>
                  </thead>
                  <tbody>
                    {items.map((v) => {
                      const p = statusPill(v.status);
                      return (
                        <tr key={v.id}>
                          <td><code>{v.provider_vm_id}</code></td>
                          <td>{v.name}</td>
                          <td><span className={p.cls}>{p.label}</span></td>
                          <td className="muted">{since(v.status_since)}</td>
                          <td>{v.cloud_account_name}</td>
                          <td>
                            {v.cluster_id && v.cluster_name ? (
                              <Link to={`/clusters/${v.cluster_id}`}>{v.cluster_name}</Link>
                            ) : v.cluster_hint ? (
                              <span className="muted">{v.cluster_hint}</span>
                            ) : (
                              <Dash />
                            )}
                          </td>
                          <td>{v.instance_type ? <code>{v.instance_type}</code> : <Dash />}</td>
                          <td>{v.power_state || <Dash />}</td>
                          <td className="muted">
                            {v.provider_creation_date ? v.provider_creation_date.slice(0, 10) : <Dash />}
                          </td>
                        </tr>
                      );
                    })}
                    {items.length === 0 && (
                      <tr>
                        <td colSpan={9} className="muted empty">
                          Nothing to show — no VM matches these filters.
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
              {cursor && (
                <div style={{ marginTop: '0.75rem' }}>
                  <button type="button" onClick={loadMore}>Load more</button>
                </div>
              )}
            </>
          );
        }}
      </AsyncView>
    </div>
  );
}

// KubeNodeVMsBanner is the "N orphan node VMs" call-out reused by the
// cloud-account admin detail page and the cluster detail page. It counts
// orphans from the filtered list itself (the summary endpoint is
// per-account only, so it can't answer a per-cluster question).
export function KubeNodeVMsBanner({
  cloudAccountId,
  clusterId,
}: {
  cloudAccountId?: string;
  clusterId?: string;
}) {
  const state = useResource(
    () =>
      api.listKubeNodeVMs({
        cloud_account_id: cloudAccountId,
        cluster_id: clusterId,
        status: ['orphan'],
        limit: 500,
      }),
    [cloudAccountId, clusterId],
  );
  if (state.status !== 'ready') return null;
  const orphans = state.data.items.length;
  if (orphans === 0) return null;
  const href =
    '/kube-node-vms' +
    (clusterId ? `?cluster_id=${clusterId}` : cloudAccountId ? `?cloud_account_id=${cloudAccountId}` : '');
  return (
    <div className="banner banner-warn">
      <span>
        {orphans} orphan node VM{orphans > 1 ? 's' : ''} on this {clusterId ? 'cluster' : 'account'} — billed for
        nothing.
      </span>
      <Link to={href}>Review</Link>
    </div>
  );
}
