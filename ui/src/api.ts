import type {
  AlertsResponse,
  DiagnosticsResponse,
  MapResponse,
  ServiceDetail,
  ServiceLogsResponse,
  ServiceTracesResponse,
  ServicesResponse,
  TraceLogsResponse,
} from './types';

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path, { headers: { accept: 'application/json' } });
  if (!res.ok) {
    // The BFF answers errors as JSON with an error and sometimes a hint, and
    // the hint is usually the useful half.
    let detail = `${res.status}`;
    try {
      const body = (await res.json()) as { error?: string; hint?: string };
      detail = [body.error, body.hint].filter(Boolean).join(' ') || detail;
    } catch {
      // Not JSON, so the status is all we have.
    }
    throw new Error(detail);
  }
  return (await res.json()) as T;
}

export const fetchServices = () => get<ServicesResponse>('/api/services');
export const fetchDiagnostics = () => get<DiagnosticsResponse>('/api/diagnostics');
export const fetchMap = () => get<MapResponse>('/api/map');
export const fetchAlerts = () => get<AlertsResponse>('/api/alerts');

export const fetchService = (name: string, namespace?: string) =>
  get<ServiceDetail>(
    `/api/services/${encodeURIComponent(name)}` +
      (namespace ? `?namespace=${encodeURIComponent(namespace)}` : ''),
  );

export const fetchServiceLogs = (name: string, traceId?: string) =>
  get<ServiceLogsResponse>(
    `/api/services/${encodeURIComponent(name)}/logs` +
      (traceId ? `?traceId=${encodeURIComponent(traceId)}` : ''),
  );

export const fetchServiceTraces = (name: string, onlyErrors = false) =>
  get<ServiceTracesResponse>(
    `/api/services/${encodeURIComponent(name)}/traces` + (onlyErrors ? '?status=error' : ''),
  );

export const fetchTraceLogs = (traceId: string) =>
  get<TraceLogsResponse>(`/api/traces/${encodeURIComponent(traceId)}/logs`);
