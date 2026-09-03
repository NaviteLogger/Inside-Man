import type { DiagnosticsResponse, ServicesResponse } from './types';

async function get<T>(path: string): Promise<T> {
  const res = await fetch(path, { headers: { accept: 'application/json' } });
  if (!res.ok) {
    throw new Error(`${path} returned ${res.status}`);
  }
  return (await res.json()) as T;
}

export const fetchServices = () => get<ServicesResponse>('/api/services');
export const fetchDiagnostics = () => get<DiagnosticsResponse>('/api/diagnostics');
