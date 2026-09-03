// Mirrors the BFF's JSON. Design doc 11.1 accepted Go for the BFF on the
// understanding that these types would be generated from an OpenAPI spec.
// Generation lands with the spec in M3. Until then this file is the contract,
// and the BFF's tests pin the field names it emits.
export type HealthStatus = 'healthy' | 'warning' | 'critical' | 'unknown';

export interface Health {
  status: HealthStatus;
  reasons?: string[];
}

export interface Pod {
  name: string;
  phase: string;
  ready: boolean;
  restarts: number;
  node?: string;
}

export interface Workload {
  namespace: string;
  kind: string;
  name: string;
  desired: number;
  ready: number;
  restarts: number;
  pods?: Pod[];
}

export interface Service {
  name: string;
  namespace: string;
  health: Health;
  requestRate: number;
  errorRatio: number;
  p95Millis: number;
  sparkline?: number[];
  workload?: Workload;
}

export interface ServicesResponse {
  services: Service[];
  window: string;
}

export interface Check {
  name: string;
  status: 'pass' | 'warn' | 'fail';
  detail: string;
  hint?: string;
}

export interface DiagnosticsResponse {
  checks: Check[];
  checkedAt: string;
}
