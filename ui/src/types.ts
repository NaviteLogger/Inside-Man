// Re-exports of the types generated from bff/openapi.yaml, so the rest of the
// UI imports plain names. Regenerate with `npm run gen-types` after changing
// the spec; CI fails when src/api-types.ts is stale.
//
// This closes the debt ADR 0001 took on. Go was chosen for the BFF partly on
// the promise that these types would be generated, and now they are.
import type { components } from './api-types';

type Schemas = components['schemas'];

export type HealthStatus = Schemas['HealthStatus'];
export type Health = Schemas['Health'];
export type Pod = Schemas['Pod'];
export type Workload = Schemas['Workload'];
export type Service = Schemas['Service'];
export type ServicesResponse = Schemas['ServicesResponse'];
export type Edge = Schemas['Edge'];
export type PodUsage = Schemas['PodUsage'];
export type TraceSummary = Schemas['TraceSummary'];
export type LogLine = Schemas['LogLine'];
export type ServiceDetail = Schemas['ServiceDetail'];
export type ServiceLogsResponse = Schemas['ServiceLogsResponse'];
export type ServiceTracesResponse = Schemas['ServiceTracesResponse'];
export type TraceLogsResponse = Schemas['TraceLogsResponse'];
export type MapNode = Schemas['MapNode'];
export type MapResponse = Schemas['MapResponse'];
export type Alert = Schemas['Alert'];
export type AlertsResponse = Schemas['AlertsResponse'];
export type Check = Schemas['Check'];
export type DiagnosticsResponse = Schemas['DiagnosticsResponse'];
