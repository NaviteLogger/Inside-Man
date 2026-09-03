import { useEffect, useState } from 'react';
import { HealthBadge } from './HealthBadge';
import { fetchService, fetchServiceLogs, fetchTraceLogs } from './api';
import { formatBytes, formatMillis, formatPercent, formatRate } from './format';
import type { Edge, LogLine, ServiceDetail as Detail } from './types';

interface Props {
  name: string;
  namespace?: string;
  onBack: () => void;
}

function Dependencies({ title, edges, side }: { title: string; edges: Edge[]; side: 'client' | 'server' }) {
  if (edges.length === 0) {
    return (
      <section>
        <h3>{title}</h3>
        <p className="muted">None seen in this window.</p>
      </section>
    );
  }
  return (
    <section>
      <h3>{title}</h3>
      <ul className="deps">
        {edges.map((e) => (
          <li key={`${e.client}->${e.server}`}>
            <span className="dep-name">{side === 'client' ? e.client : e.server}</span>
            <span className="muted">{formatRate(e.requestRate)}</span>
            <span className={e.errorRatio > 0 ? 'has-errors' : 'muted'}>
              {formatPercent(e.errorRatio)}
            </span>
          </li>
        ))}
      </ul>
    </section>
  );
}

export function ServiceDetail({ name, namespace, onBack }: Props) {
  const [detail, setDetail] = useState<Detail | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [lines, setLines] = useState<LogLine[]>([]);
  const [pinnedTrace, setPinnedTrace] = useState<string | null>(null);

  useEffect(() => {
    setDetail(null);
    setError(null);
    fetchService(name, namespace).then(setDetail).catch((e: Error) => setError(e.message));
  }, [name, namespace]);

  useEffect(() => {
    // With no trace pinned this is the service's own tail. Pinning one narrows
    // it to that request, which is the three-clicks path the design doc asks
    // for: services, service, failing trace, its logs.
    const load = pinnedTrace ? fetchTraceLogs(pinnedTrace) : fetchServiceLogs(name);
    load.then((r) => setLines(r.lines)).catch(() => setLines([]));
  }, [name, pinnedTrace]);

  if (error) {
    return (
      <>
        <button type="button" className="back" onClick={onBack}>← Services</button>
        <p className="error">{error}</p>
      </>
    );
  }
  if (!detail) return <p className="muted">Loading…</p>;

  const pods = detail.workload?.pods ?? [];

  return (
    <>
      <button type="button" className="back" onClick={onBack}>← Services</button>

      <div className="detail-head">
        <h2>{detail.name}</h2>
        <HealthBadge health={detail.health} />
        <span className="muted">{detail.namespace}</span>
        <span className="muted detail-window">over {detail.window}</span>
      </div>

      {detail.health.reasons?.length ? (
        <ul className="reasons">
          {detail.health.reasons.map((r) => <li key={r}>{r}</li>)}
        </ul>
      ) : null}

      <dl className="red">
        <div><dt>Rate</dt><dd>{formatRate(detail.requestRate)}</dd></div>
        <div><dt>Errors</dt><dd className={detail.errorRatio > 0 ? 'has-errors' : ''}>
          {formatPercent(detail.errorRatio)}
        </dd></div>
        <div><dt>p95</dt><dd>{formatMillis(detail.p95Millis)}</dd></div>
      </dl>

      {detail.links ? (
        <p className="links">
          {/* Linking out keeps a waterfall and a log explorer off our plate.
              See docs/decisions/0005-embed-grafana-drilldown-for-traces-and-logs.md */}
          <a href={detail.links.traces} target="_blank" rel="noreferrer">Traces in Grafana ↗</a>
          <a href={detail.links.logs} target="_blank" rel="noreferrer">Logs in Grafana ↗</a>
        </p>
      ) : null}

      <div className="detail-grid">
        <Dependencies title="Called by" edges={detail.inbound} side="client" />
        <Dependencies title="Calls" edges={detail.outbound} side="server" />
      </div>

      <section>
        <h3>Pods</h3>
        {pods.length === 0 ? (
          <p className="muted">No pods matched this service.</p>
        ) : (
          <table className="pods">
            <thead>
              <tr>
                <th scope="col">Pod</th><th scope="col">Phase</th>
                <th scope="col" className="num">Restarts</th>
                <th scope="col" className="num">CPU</th>
                <th scope="col" className="num">Memory</th>
              </tr>
            </thead>
            <tbody>
              {pods.map((p) => {
                const usage = detail.resources?.[p.name];
                return (
                  <tr key={p.name} className={p.ready ? '' : 'row-warning'}>
                    <th scope="row">{p.name}</th>
                    <td>{p.ready ? p.phase : `${p.phase} (not ready)`}</td>
                    <td className={`num ${p.restarts > 0 ? 'restarts' : ''}`}>{p.restarts}</td>
                    <td className="num">{usage ? `${usage.cpuMillis.toFixed(1)}m` : '–'}</td>
                    <td className="num">{usage ? formatBytes(usage.memBytes) : '–'}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </section>

      <section>
        <h3>Failing traces</h3>
        {detail.errorTraces.length === 0 ? (
          <p className="muted">No failing traces in the last hour.</p>
        ) : (
          <ul className="traces">
            {detail.errorTraces.map((t) => (
              <li key={t.traceId}>
                <button
                  type="button"
                  className={pinnedTrace === t.traceId ? 'trace active' : 'trace'}
                  onClick={() => setPinnedTrace(pinnedTrace === t.traceId ? null : t.traceId ?? null)}
                >
                  <code>{t.traceId?.slice(0, 16)}</code>
                  <span className="muted">{t.rootServiceName} {t.rootTraceName}</span>
                  <span className="muted">{t.durationMillis}ms</span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section>
        <h3>
          {pinnedTrace ? 'Logs for the selected trace' : 'Recent logs'}
          {pinnedTrace ? (
            <button type="button" className="clear" onClick={() => setPinnedTrace(null)}>
              show all
            </button>
          ) : null}
        </h3>
        {lines.length === 0 ? (
          <p className="muted">No log lines.</p>
        ) : (
          <ol className="logs">
            {lines.map((l, i) => (
              <li key={`${l.timestamp}-${i}`}>
                <span className="log-service">{l.service}</span>
                <code>{l.line}</code>
              </li>
            ))}
          </ol>
        )}
      </section>
    </>
  );
}
