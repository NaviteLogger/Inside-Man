import { useCallback, useEffect, useState } from 'react';
import { Diagnostics } from './Diagnostics';
import { ServiceDetail } from './ServiceDetail';
import { ServicesTable } from './ServicesTable';
import { fetchServices } from './api';
import type { ServicesResponse } from './types';

const REFRESH_MS = 15_000;

type Route =
  | { view: 'services' }
  | { view: 'diagnostics' }
  | { view: 'service'; name: string; namespace?: string };

// Routes live in the URL so every screen is shareable, per design doc 5.3.
function routeFromLocation(): Route {
  const { pathname, searchParams } = new URL(window.location.href);
  if (pathname.startsWith('/diagnostics')) return { view: 'diagnostics' };

  const match = /^\/services\/([^/]+)/.exec(pathname);
  if (match) {
    return {
      view: 'service',
      name: decodeURIComponent(match[1]),
      namespace: searchParams.get('namespace') ?? undefined,
    };
  }
  return { view: 'services' };
}

function hrefFor(route: Route): string {
  switch (route.view) {
    case 'diagnostics':
      return '/diagnostics';
    case 'service':
      return `/services/${encodeURIComponent(route.name)}` +
        (route.namespace ? `?namespace=${encodeURIComponent(route.namespace)}` : '');
    default:
      return '/';
  }
}

export function App() {
  const [route, setRoute] = useState<Route>(routeFromLocation);
  const [data, setData] = useState<ServicesResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const onPop = () => setRoute(routeFromLocation());
    window.addEventListener('popstate', onPop);
    return () => window.removeEventListener('popstate', onPop);
  }, []);

  const navigate = useCallback((next: Route) => {
    window.history.pushState(null, '', hrefFor(next));
    setRoute(next);
  }, []);

  const load = useCallback(() => {
    fetchServices()
      .then((d) => { setData(d); setError(null); })
      .catch((e: Error) => setError(e.message));
  }, []);

  useEffect(() => {
    if (route.view !== 'services') return;
    load();
    const id = setInterval(load, REFRESH_MS);
    return () => clearInterval(id);
  }, [route.view, load]);

  return (
    <div className="app">
      <header>
        <h1>
          <button type="button" className="home" onClick={() => navigate({ view: 'services' })}>
            Inside Man
          </button>
        </h1>
        <nav>
          <button
            type="button"
            className={route.view === 'services' || route.view === 'service' ? 'active' : ''}
            onClick={() => navigate({ view: 'services' })}
          >
            Services
          </button>
          <button
            type="button"
            className={route.view === 'diagnostics' ? 'active' : ''}
            onClick={() => navigate({ view: 'diagnostics' })}
          >
            Diagnostics
          </button>
        </nav>
        {route.view === 'services' && data ? (
          <span className="muted window">over {data.window}</span>
        ) : null}
      </header>

      <main>
        {route.view === 'diagnostics' ? (
          <Diagnostics />
        ) : route.view === 'service' ? (
          <ServiceDetail
            name={route.name}
            namespace={route.namespace}
            onBack={() => navigate({ view: 'services' })}
          />
        ) : error ? (
          <p className="error">
            Could not load services: {error}. The <a href="/diagnostics">diagnostics page</a> may
            explain why.
          </p>
        ) : data ? (
          <ServicesTable
            services={data.services}
            onSelect={(name, namespace) => navigate({ view: 'service', name, namespace })}
          />
        ) : (
          <p className="muted">Loading…</p>
        )}
      </main>
    </div>
  );
}
