import { describe, expect, it } from 'vitest';
import { http, HttpResponse } from 'msw';
import { screen, waitFor } from '@testing-library/react';
import KubeNodeVMs from './KubeNodeVMs';
import { renderWithRouter } from '../test/render';
import { server } from '../test/server';

const row = {
  id: '11111111-1111-1111-1111-111111111111',
  cloud_account_id: '22222222-2222-2222-2222-222222222222',
  cloud_account_name: 'zex-preprod-std',
  provider_vm_id: 'i-04d9545b',
  cluster_tag: 'main-zex-preprod-ebe6fc0e',
  node_name_tag: '',
  cluster_hint: 'main-zex-preprod',
  name: 'main-zex-preprod-md-worker-eu-west-2b-vx2lp-984mp',
  instance_type: 'tinav7.c8r32p1',
  power_state: 'running',
  zone: 'eu-west-2b',
  vpc_id: 'vpc-ce8ae24f',
  image_id: 'ami-1',
  image_name: 'img',
  provider_creation_date: '2025-10-27T09:12:44Z',
  node_id: null,
  cluster_id: '33333333-3333-3333-3333-333333333333',
  cluster_name: 'zex-preprod-main-std',
  status: 'orphan',
  status_since: '2025-10-28T09:12:44Z',
  first_seen_at: '2026-09-14T10:00:00Z',
  last_seen_at: '2026-09-14T12:00:00Z',
};

describe('KubeNodeVMs page', () => {
  it('lists orphan and unknown VMs by default with a status badge and cluster link', async () => {
    let requestedUrl = '';
    server.use(
      http.get('/v1/kube-node-vms', ({ request }) => {
        requestedUrl = request.url;
        return HttpResponse.json({ items: [row], next_cursor: null });
      }),
      http.get('/v1/kube-node-vms/summary', () =>
        HttpResponse.json({ rows: [{ cloud_account_id: row.cloud_account_id, cloud_account_name: 'zex-preprod-std', status: 'orphan', count: 13, vcpu: 104, memory_gib: 416 }] }),
      ),
    );
    renderWithRouter(<KubeNodeVMs />);
    await waitFor(() => expect(screen.getByText('i-04d9545b')).toBeInTheDocument());
    expect(screen.getByText('Orphan')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'zex-preprod-main-std' })).toHaveAttribute('href', `/clusters/${row.cluster_id}`);
    expect(requestedUrl).not.toContain('status=');
    expect(screen.getByText(/13/)).toBeInTheDocument();
  });
});
