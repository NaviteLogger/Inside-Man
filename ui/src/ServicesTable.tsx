import { HealthBadge } from './HealthBadge';
import { Sparkline } from './Sparkline';
import { formatMillis, formatPercent, formatRate } from './format';
import type { Service } from './types';

export function ServicesTable({ services }: { services: Service[] }) {
  if (services.length === 0) {
    // Design doc 5.3: empty states teach.
    return (
      <div className="empty">
        <h2>No services are reporting yet</h2>
        <p>
          Annotate a pod with <code>instrumentation.opentelemetry.io/inject-java</code> (or
          <code>-nodejs</code>, <code>-python</code>, <code>-dotnet</code>) and send it some
          traffic.
        </p>
        <p>
          If you expected something here, the <a href="/diagnostics">diagnostics page</a> checks
          whether the join key is intact.
        </p>
      </div>
    );
  }

  return (
    <table className="services">
      <thead>
        <tr>
          <th scope="col">Health</th>
          <th scope="col">Service</th>
          <th scope="col">Namespace</th>
          <th scope="col" className="num">Rate</th>
          <th scope="col" className="num">Errors</th>
          <th scope="col" className="num">p95</th>
          <th scope="col">Pods</th>
          <th scope="col">Last hour</th>
        </tr>
      </thead>
      <tbody>
        {services.map((s) => (
          <tr key={`${s.namespace}/${s.name}`} className={`row-${s.health.status}`}>
            <td><HealthBadge health={s.health} /></td>
            <th scope="row">{s.name}</th>
            <td className="muted">{s.namespace || '–'}</td>
            <td className="num">{formatRate(s.requestRate)}</td>
            <td className={`num ${s.errorRatio > 0 ? 'has-errors' : ''}`}>
              {formatPercent(s.errorRatio)}
            </td>
            <td className="num">{formatMillis(s.p95Millis)}</td>
            <td className="muted">
              {s.workload ? `${s.workload.ready}/${s.workload.desired}` : '–'}
              {s.workload && s.workload.restarts > 0 ? (
                <span className="restarts" title={`${s.workload.restarts} restarts`}>
                  {' '}↻{s.workload.restarts}
                </span>
              ) : null}
            </td>
            <td><Sparkline values={s.sparkline ?? []} label={`${s.name} request rate`} /></td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
