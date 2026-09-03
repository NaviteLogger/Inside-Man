export function formatRate(perSecond: number): string {
  if (perSecond >= 100) return `${Math.round(perSecond)}/s`;
  if (perSecond >= 1) return `${perSecond.toFixed(1)}/s`;
  return `${perSecond.toFixed(2)}/s`;
}

export function formatPercent(ratio: number): string {
  const pct = ratio * 100;
  if (pct === 0) return '0%';
  if (pct < 0.01) return '<0.01%';
  return `${pct.toFixed(2)}%`;
}

export function formatMillis(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '–';
  if (ms >= 1000) return `${(ms / 1000).toFixed(2)}s`;
  if (ms >= 10) return `${Math.round(ms)}ms`;
  return `${ms.toFixed(1)}ms`;
}
