import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ServicesTable } from './ServicesTable';
import { formatBytes, formatMillis, formatPercent, formatRate } from './format';
import type { Service } from './types';

afterEach(() => vi.restoreAllMocks());

function service(over: Partial<Service> = {}): Service {
  return {
    name: 'checkout',
    namespace: 'shop',
    health: { status: 'healthy' },
    requestRate: 2.5,
    errorRatio: 0,
    p95Millis: 42,
    workload: { namespace: 'shop', kind: 'Deployment', name: 'checkout', desired: 2, ready: 2, restarts: 0 },
    ...over,
  };
}

describe('ServicesTable', () => {
  it('teaches what to do when nothing is reporting', () => {
    render(<ServicesTable services={[]} />);
    expect(screen.getByText(/No services are reporting yet/)).toBeInTheDocument();
    expect(screen.getByText(/instrumentation.opentelemetry.io/)).toBeInTheDocument();
  });

  it('renders a service row with its numbers', () => {
    render(<ServicesTable services={[service()]} />);
    expect(screen.getByRole('row', { name: /checkout/ })).toBeInTheDocument();
    expect(screen.getByText('2/2')).toBeInTheDocument();
    expect(screen.getByText('Healthy')).toBeInTheDocument();
  });

  it('shows the reason behind a non-healthy status', async () => {
    render(
      <ServicesTable
        services={[service({ health: { status: 'critical', reasons: ['error rate 20.00% exceeds 5.00%'] } })]}
      />,
    );
    await waitFor(() => {
      expect(screen.getByTitle('error rate 20.00% exceeds 5.00%')).toBeInTheDocument();
    });
  });

  it('flags restarts so a flapping pod is visible on the list', () => {
    render(<ServicesTable services={[service({
      workload: { namespace: 'shop', kind: 'Deployment', name: 'checkout', desired: 2, ready: 1, restarts: 4 },
    })]} />);
    expect(screen.getByTitle('4 restarts')).toBeInTheDocument();
  });
});

describe('formatting', () => {
  it('keeps small rates legible', () => {
    expect(formatRate(0.05)).toBe('0.05/s');
    expect(formatRate(2.51)).toBe('2.5/s');
    expect(formatRate(140)).toBe('140/s');
  });

  it('distinguishes zero errors from a very small error rate', () => {
    expect(formatPercent(0)).toBe('0%');
    expect(formatPercent(0.00001)).toBe('<0.01%');
    expect(formatPercent(0.2)).toBe('20.00%');
  });

  it('shows a dash when there is no p95 to show', () => {
    expect(formatMillis(0)).toBe('–');
    expect(formatMillis(NaN)).toBe('–');
    expect(formatMillis(1500)).toBe('1.50s');
  });
});

describe('ServicesTable navigation', () => {
  it('makes each service name a way into its detail screen', async () => {
    const onSelect = vi.fn();
    render(<ServicesTable services={[service()]} onSelect={onSelect} />);
    screen.getByRole('button', { name: 'checkout' }).click();
    await waitFor(() => expect(onSelect).toHaveBeenCalledWith('checkout', 'shop'));
  });

  it('renders plain text when there is nowhere to navigate', () => {
    render(<ServicesTable services={[service()]} />);
    expect(screen.queryByRole('button', { name: 'checkout' })).toBeNull();
  });
});

describe('formatBytes', () => {
  it('switches unit at a gibibyte', () => {
    expect(formatBytes(0)).toBe('–');
    expect(formatBytes(85 * 1024 * 1024)).toBe('85MiB');
    expect(formatBytes(2.5 * 1024 * 1024 * 1024)).toBe('2.50GiB');
  });
});
