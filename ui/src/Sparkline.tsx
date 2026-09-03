// A 31-point sparkline is a polyline, so it stays inline SVG. Design doc 6
// picks ECharts or uPlot for dense time series, and that arrives in M3 with the
// real charts on the service detail screen.
interface Props {
  values: number[];
  width?: number;
  height?: number;
  label: string;
}

export function Sparkline({ values, width = 90, height = 22, label }: Props) {
  if (values.length < 2) {
    return <span className="sparkline-empty" aria-label={`${label}: not enough data`} />;
  }

  const max = Math.max(...values);
  const min = Math.min(...values);
  const span = max - min || 1;
  const step = width / (values.length - 1);

  const points = values
    .map((v, i) => `${(i * step).toFixed(1)},${(height - ((v - min) / span) * height).toFixed(1)}`)
    .join(' ');

  return (
    <svg
      className="sparkline"
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      role="img"
      aria-label={`${label}: ${values.length} points, peak ${max.toFixed(2)} per second`}
    >
      <polyline points={points} fill="none" strokeWidth="1.5" vectorEffect="non-scaling-stroke" />
    </svg>
  );
}
