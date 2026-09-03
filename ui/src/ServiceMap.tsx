import { useEffect, useMemo, useState } from 'react';
import { fetchMap } from './api';
import { formatPercent, formatRate } from './format';
import type { MapNode, MapResponse } from './types';

interface Placed extends MapNode {
  x: number;
  y: number;
}

// Nodes are laid out in a ring. A force simulation would need a graph library,
// and design doc 6 only reaches for React Flow or Cytoscape once graphs get
// large. A ring reads well at demo scale and costs nothing.
function place(nodes: MapNode[], width: number, height: number): Placed[] {
  const cx = width / 2;
  const cy = height / 2;
  const radius = Math.min(width, height) / 2 - 70;
  return nodes.map((n, i) => {
    const angle = (2 * Math.PI * i) / Math.max(nodes.length, 1) - Math.PI / 2;
    return { ...n, x: cx + radius * Math.cos(angle), y: cy + radius * Math.sin(angle) };
  });
}

export function ServiceMap({ onSelect }: { onSelect?: (name: string, namespace?: string) => void }) {
  const [data, setData] = useState<MapResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const load = () => fetchMap().then((d) => { setData(d); setError(null); })
      .catch((e: Error) => setError(e.message));
    load();
    const id = setInterval(load, 15_000);
    return () => clearInterval(id);
  }, []);

  const width = 720;
  const height = 420;
  const placed = useMemo(() => place(data?.nodes ?? [], width, height), [data]);
  const byName = useMemo(() => new Map(placed.map((n) => [n.name, n])), [placed]);

  if (error) return <p className="error">Could not load the map: {error}</p>;
  if (!data) return <p className="muted">Loading…</p>;

  if (data.nodes.length === 0) {
    return (
      <div className="empty">
        <h2>Nothing to map yet</h2>
        <p>The map is built from the service graph, which needs traced calls between services.</p>
      </div>
    );
  }

  return (
    <>
      <p className="muted">
        Dependencies over {data.window}. Node colour is the same health the services list shows.
      </p>
      <svg className="map" viewBox={`0 0 ${width} ${height}`} role="img" aria-label="Service dependency map">
        <defs>
          <marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5"
                  markerWidth="6" markerHeight="6" orient="auto-start-reverse">
            <path d="M 0 0 L 10 5 L 0 10 z" />
          </marker>
        </defs>

        {data.edges.map((e) => {
          const from = byName.get(e.client);
          const to = byName.get(e.server);
          if (!from || !to) return null;
          return (
            <g key={`${e.client}->${e.server}`} className={e.errorRatio > 0 ? 'edge has-errors' : 'edge'}>
              <line x1={from.x} y1={from.y} x2={to.x} y2={to.y} markerEnd="url(#arrow)" />
              <title>
                {`${e.client} to ${e.server}: ${formatRate(e.requestRate)}, ${formatPercent(e.errorRatio)} errors`}
              </title>
            </g>
          );
        })}

        {placed.map((n) => (
          <g
            key={n.name}
            className={`node node-${n.health.status}`}
            transform={`translate(${n.x},${n.y})`}
            onClick={() => n.namespace && onSelect?.(n.name, n.namespace)}
            role={n.namespace ? 'button' : undefined}
            tabIndex={n.namespace ? 0 : undefined}
          >
            <circle r={14} />
            <text y={30} textAnchor="middle">{n.name}</text>
            <title>
              {`${n.name}: ${n.health.status}` +
                (n.health.reasons?.length ? `\n${n.health.reasons.join('\n')}` : '')}
            </title>
          </g>
        ))}
      </svg>
    </>
  );
}
