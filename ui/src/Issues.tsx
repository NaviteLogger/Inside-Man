import { useEffect, useState } from 'react';
import { fetchAlerts } from './api';
import type { Alert, AlertsResponse } from './types';

function since(startsAt: string): string {
  const ms = Date.now() - new Date(startsAt).getTime();
  if (!Number.isFinite(ms) || ms < 0) return '';
  const mins = Math.floor(ms / 60000);
  if (mins < 60) return `${mins}m`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ${mins % 60}m`;
  return `${Math.floor(hours / 24)}d ${hours % 24}h`;
}

function AlertRow({ alert }: { alert: Alert }) {
  return (
    <li className={`alert alert-${alert.severity || 'unknown'}`}>
      <span className={`badge badge-${alert.severity === 'critical' ? 'critical' : 'warning'}`}>
        {alert.severity || 'unknown'}
      </span>
      <div className="alert-body">
        <strong>{alert.name}</strong>
        {alert.summary ? <div>{alert.summary}</div> : null}
        {alert.description ? <div className="muted">{alert.description}</div> : null}
      </div>
      <span className="muted alert-since" title={alert.startsAt}>
        {since(alert.startsAt)}
      </span>
    </li>
  );
}

export function Issues({ onSelect }: { onSelect?: (name: string, namespace?: string) => void }) {
  const [data, setData] = useState<AlertsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const load = () => fetchAlerts().then((d) => { setData(d); setError(null); })
      .catch((e: Error) => setError(e.message));
    load();
    const id = setInterval(load, 15_000);
    return () => clearInterval(id);
  }, []);

  if (error) return <p className="error">Could not load alerts: {error}</p>;
  if (!data) return <p className="muted">Loading…</p>;

  if (data.alerts.length === 0) {
    return (
      <div className="empty">
        <h2>Nothing is firing</h2>
        <p>
          Alerts come from the rules the chart ships, which mirror the health model, so a red badge
          on the services list becomes an alert here once it has held for long enough.
        </p>
      </div>
    );
  }

  // Grouped by service, per design doc 5.1. Alerts naming no service land under
  // an empty key and are shown as cluster-wide.
  const groups = Object.entries(data.byService).sort(([a], [b]) => {
    if (a === '') return 1;
    if (b === '') return -1;
    return a.localeCompare(b);
  });

  return (
    <>
      <p className="muted">
        {data.alerts.length} firing across {groups.length} {groups.length === 1 ? 'group' : 'groups'}.
      </p>
      {groups.map(([service, list]) => (
        <section key={service || 'cluster'}>
          <h3>
            {service ? (
              onSelect ? (
                <button type="button" className="link" onClick={() => onSelect(service, list[0]?.namespace)}>
                  {service}
                </button>
              ) : (
                service
              )
            ) : (
              'Cluster-wide'
            )}
          </h3>
          <ul className="alerts">
            {list.map((a) => <AlertRow key={`${a.name}-${a.startsAt}`} alert={a} />)}
          </ul>
        </section>
      ))}
    </>
  );
}
