import { useEffect, useState } from 'react';
import { fetchDiagnostics } from './api';
import type { DiagnosticsResponse } from './types';

const ICONS = { pass: '✓', warn: '!', fail: '✗' } as const;

export function Diagnostics() {
  const [data, setData] = useState<DiagnosticsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    fetchDiagnostics().then(setData).catch((e: Error) => setError(e.message));
  }, []);

  if (error) return <p className="error">Diagnostics unavailable: {error}</p>;
  if (!data) return <p className="muted">Checking…</p>;

  return (
    <>
      <p className="muted">
        Whether the join key is intact. Start here when a screen is unexpectedly empty.
      </p>
      <ul className="checks">
        {data.checks.map((c) => (
          <li key={c.name} className={`check check-${c.status}`}>
            <span className="check-icon" aria-hidden="true">{ICONS[c.status]}</span>
            <div>
              <strong>{c.name}</strong>
              <div className="muted">{c.detail}</div>
              {c.hint ? <div className="hint">{c.hint}</div> : null}
            </div>
          </li>
        ))}
      </ul>
    </>
  );
}
