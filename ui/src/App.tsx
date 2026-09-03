import { useCallback, useEffect, useState } from 'react';
import { Diagnostics } from './Diagnostics';
import { ServicesTable } from './ServicesTable';
import { fetchServices } from './api';
import type { ServicesResponse } from './types';

const REFRESH_MS = 15_000;

type View = 'services' | 'diagnostics';

// The view lives in the URL so every screen is shareable, per design doc 5.3.
function viewFromLocation(): View {
  return window.location.pathname.startsWith('/diagnostics') ? 'diagnostics' : 'services';
}

export function App() {
  const [view, setView] = useState<View>(viewFromLocation);
  const [data, setData] = useState<ServicesResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const onPop = () => setView(viewFromLocation());
    window.addEventListener('popstate', onPop);
    return () => window.removeEventListener('popstate', onPop);
  }, []);

  const navigate = useCallback((next: View) => {
    window.history.pushState(null, '', next === 'services' ? '/' : '/diagnostics');
    setView(next);
  }, []);

  const load = useCallback(() => {
    fetchServices()
      .then((d) => { setData(d); setError(null); })
      .catch((e: Error) => setError(e.message));
  }, []);

  useEffect(() => {
    if (view !== 'services') return;
    load();
    const id = setInterval(load, REFRESH_MS);
    return () => clearInterval(id);
  }, [view, load]);

  return (
    <div className="app">
      <header>
        <h1>Inside Man</h1>
        <nav>
          <button
            type="button"
            className={view === 'services' ? 'active' : ''}
            onClick={() => navigate('services')}
          >
            Services
          </button>
          <button
            type="button"
            className={view === 'diagnostics' ? 'active' : ''}
            onClick={() => navigate('diagnostics')}
          >
            Diagnostics
          </button>
        </nav>
        {view === 'services' && data ? (
          <span className="muted window">over {data.window}</span>
        ) : null}
      </header>

      <main>
        {view === 'diagnostics' ? (
          <Diagnostics />
        ) : error ? (
          <p className="error">
            Could not load services: {error}. The <a href="/diagnostics">diagnostics page</a> may
            explain why.
          </p>
        ) : data ? (
          <ServicesTable services={data.services} />
        ) : (
          <p className="muted">Loading…</p>
        )}
      </main>
    </div>
  );
}
