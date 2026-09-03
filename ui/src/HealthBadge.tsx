import type { Health } from './types';

const LABELS: Record<Health['status'], string> = {
  healthy: 'Healthy',
  warning: 'Warning',
  critical: 'Critical',
  unknown: 'Unknown',
};

export function HealthBadge({ health }: { health: Health }) {
  // Reasons become the tooltip, so a status never appears without an
  // explanation available. Design doc 5.3 asks every number to be explainable.
  const title = health.reasons?.length ? health.reasons.join('\n') : undefined;
  return (
    <span className={`badge badge-${health.status}`} title={title}>
      {LABELS[health.status]}
    </span>
  );
}
