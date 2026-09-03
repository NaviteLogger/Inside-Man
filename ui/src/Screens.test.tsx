import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Diagnostics } from './Diagnostics';
import { Issues } from './Issues';
import { ServiceDetail } from './ServiceDetail';
import { ServiceMap } from './ServiceMap';

function mockFetch(routes: Record<string, unknown>) {
  vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
    const url = String(input);
    // Longest prefix wins, so /api/services/x/logs does not match /api/services/x.
    const key = Object.keys(routes)
      .filter((k) => url.startsWith(k))
      .sort((a, b) => b.length - a.length)[0];
    if (!key) return Promise.resolve(new Response('{}', { status: 404 }));
    return Promise.resolve(
      new Response(JSON.stringify(routes[key]), {
        status: 200,
        headers: { 'content-type': 'application/json' },
      }),
    );
  }));
}

beforeEach(() => vi.useFakeTimers({ shouldAdvanceTime: true }));
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('Diagnostics', () => {
  it('shows each check with its hint, so an empty screen is explainable', async () => {
    mockFetch({
      '/api/diagnostics': {
        checkedAt: '2026-09-03T10:00:00Z',
        checks: [
          { name: 'metrics store reachable', status: 'pass', detail: 'ok' },
          {
            name: 'services resolve to workloads',
            status: 'warn',
            detail: 'no Deployment found for shop/catalog',
            hint: 'service.name should equal the Deployment name.',
          },
        ],
      },
    });
    render(<Diagnostics />);
    await waitFor(() => expect(screen.getByText('services resolve to workloads')).toBeInTheDocument());
    expect(screen.getByText(/service.name should equal the Deployment name/)).toBeInTheDocument();
  });

  it('reports its own failure, leaving no blank panel', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response('{}', { status: 502 }))));
    render(<Diagnostics />);
    await waitFor(() => expect(screen.getByText(/Diagnostics unavailable/)).toBeInTheDocument());
  });
});

describe('Issues', () => {
  const alert = {
    name: 'ServiceErrorRateCritical',
    service: 'checkout',
    severity: 'critical',
    summary: 'checkout is failing more than 5% of requests',
    startsAt: new Date(Date.now() - 90 * 60 * 1000).toISOString(),
  };

  it('groups alerts by service and shows how long they have been firing', async () => {
    mockFetch({ '/api/alerts': { alerts: [alert], byService: { checkout: [alert] } } });
    render(<Issues />);
    await waitFor(() => expect(screen.getByText('ServiceErrorRateCritical')).toBeInTheDocument());
    expect(screen.getByRole('heading', { name: 'checkout' })).toBeInTheDocument();
    expect(screen.getByText('1h 30m')).toBeInTheDocument();
  });

  it('shows an alert that names no service as cluster-wide', async () => {
    const clusterAlert = { ...alert, service: '' };
    mockFetch({ '/api/alerts': { alerts: [clusterAlert], byService: { '': [clusterAlert] } } });
    render(<Issues />);
    await waitFor(() => expect(screen.getByRole('heading', { name: 'Cluster-wide' })).toBeInTheDocument());
  });

  it('says so plainly when nothing is firing', async () => {
    mockFetch({ '/api/alerts': { alerts: [], byService: {} } });
    render(<Issues />);
    await waitFor(() => expect(screen.getByText('Nothing is firing')).toBeInTheDocument());
  });
});

describe('ServiceMap', () => {
  it('draws a node per service and an edge per dependency', async () => {
    mockFetch({
      '/api/map': {
        window: '5m0s',
        nodes: [
          { name: 'frontend', namespace: 'demo', health: { status: 'healthy' }, requestRate: 1, errorRatio: 0 },
          { name: 'api', namespace: 'demo', health: { status: 'critical', reasons: ['error rate high'] }, requestRate: 1, errorRatio: 0.2 },
        ],
        edges: [{ client: 'frontend', server: 'api', requestRate: 1, errorRatio: 0.2 }],
      },
    });
    const { container } = render(<ServiceMap />);
    await waitFor(() => expect(screen.getByText('frontend')).toBeInTheDocument());
    expect(container.querySelectorAll('g.node')).toHaveLength(2);
    expect(container.querySelectorAll('g.edge')).toHaveLength(1);
    // The failing edge is marked, so a broken dependency is visible.
    expect(container.querySelector('g.edge.has-errors')).not.toBeNull();
    expect(container.querySelector('g.node-critical')).not.toBeNull();
  });

  it('teaches what the map needs when the graph is empty', async () => {
    mockFetch({ '/api/map': { window: '5m0s', nodes: [], edges: [] } });
    render(<ServiceMap />);
    await waitFor(() => expect(screen.getByText('Nothing to map yet')).toBeInTheDocument());
  });
});

describe('ServiceDetail', () => {
  const detail = {
    name: 'checkout',
    namespace: 'shop',
    health: { status: 'critical', reasons: ['error rate 20.00% exceeds 5.00%'] },
    requestRate: 2,
    errorRatio: 0.2,
    p95Millis: 120,
    window: '5m0s',
    workload: {
      namespace: 'shop', kind: 'Deployment', name: 'checkout', desired: 2, ready: 1, restarts: 3,
      pods: [{ name: 'checkout-abc', phase: 'Running', ready: true, restarts: 3 }],
    },
    resources: { 'checkout-abc': { cpuMillis: 42.5, memBytes: 104857600 } },
    inbound: [{ client: 'frontend', server: 'checkout', requestRate: 2, errorRatio: 0 }],
    outbound: [{ client: 'checkout', server: 'payments', requestRate: 1, errorRatio: 0.5 }],
    errorTraces: [{ traceId: '4bf92f3577b34da6a3ce929d0e0e4736', rootServiceName: 'frontend', rootTraceName: 'GET', durationMillis: 30 }],
    links: { logs: 'http://g/logs', traces: 'http://g/traces', exploreLogs: 'http://g/el', exploreTraces: 'http://g/et' },
  };

  it('shows the reason behind a critical status', async () => {
    mockFetch({
      '/api/services/checkout': detail,
      '/api/services/checkout/logs': { service: 'checkout', lines: [] },
    });
    render(<ServiceDetail name="checkout" namespace="shop" onBack={() => {}} />);
    await waitFor(() => expect(screen.getByText('error rate 20.00% exceeds 5.00%')).toBeInTheDocument());
  });

  it('shows pods with their CPU and memory', async () => {
    mockFetch({
      '/api/services/checkout': detail,
      '/api/services/checkout/logs': { service: 'checkout', lines: [] },
    });
    render(<ServiceDetail name="checkout" namespace="shop" onBack={() => {}} />);
    await waitFor(() => expect(screen.getByText('checkout-abc')).toBeInTheDocument());
    expect(screen.getByText('42.5m')).toBeInTheDocument();
    expect(screen.getByText('100MiB')).toBeInTheDocument();
  });

  it('links out to Grafana, keeping a waterfall off our plate', async () => {
    mockFetch({
      '/api/services/checkout': detail,
      '/api/services/checkout/logs': { service: 'checkout', lines: [] },
    });
    render(<ServiceDetail name="checkout" namespace="shop" onBack={() => {}} />);
    await waitFor(() => {
      expect(screen.getByRole('link', { name: /Traces in Grafana/ })).toHaveAttribute('href', 'http://g/traces');
    });
  });

  it('surfaces a load failure with the hint the API gave', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(
      new Response(JSON.stringify({ error: 'no span metrics for service checkout', hint: 'It may not be instrumented.' }),
        { status: 404, headers: { 'content-type': 'application/json' } }),
    )));
    render(<ServiceDetail name="checkout" onBack={() => {}} />);
    await waitFor(() => expect(screen.getByText(/It may not be instrumented/)).toBeInTheDocument());
  });
});
