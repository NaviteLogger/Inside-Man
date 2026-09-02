// Entry point of the demo chain: frontend (Node) -> api (Python) -> backend (Java).
// No dependencies, so the OpenTelemetry Node agent instruments the built-in
// http module and nothing else is needed.
const http = require('http');

// The agent mounts its own node_modules, and NODE_PATH (set in the manifest)
// makes them resolvable here. This is the only tracing-aware code in the demo,
// and it exists so log lines carry the trace they belong to. Spans themselves
// need none of it.
let otel = null;
try {
  otel = require('@opentelemetry/api');
} catch {
  // Running without the agent, so logs simply carry no trace id.
}

function traceContext() {
  const span = otel && otel.trace.getActiveSpan();
  if (!span) return {};
  const { traceId, spanId } = span.spanContext();
  return traceId ? { trace_id: traceId, span_id: spanId } : {};
}

const API = process.env.API_URL || 'http://demo-api:8080';
const PORT = Number(process.env.PORT || 8080);

function log(level, msg, extra = {}) {
  // JSON on stdout, which is what the log pipeline expects.
  console.log(JSON.stringify({ level, msg, service: 'frontend', ...traceContext(), ...extra }));
}

function callApi(path) {
  return new Promise((resolve, reject) => {
    const req = http.get(`${API}${path}`, (res) => {
      let body = '';
      res.on('data', (c) => (body += c));
      res.on('end', () => resolve({ status: res.statusCode, body }));
    });
    req.on('error', reject);
    req.setTimeout(5000, () => req.destroy(new Error('api timeout')));
  });
}

http.createServer(async (req, res) => {
  const url = new URL(req.url, `http://localhost:${PORT}`);

  if (url.pathname === '/healthz') {
    res.writeHead(200).end('ok');
    return;
  }

  // /checkout succeeds, /checkout?fail=1 propagates a failure from downstream.
  if (url.pathname === '/checkout') {
    const downstream = url.searchParams.get('fail') === '1' ? '/order?fail=1' : '/order';
    try {
      const r = await callApi(downstream);
      if (r.status >= 500) {
        log('error', 'checkout failed downstream', { status: r.status });
        res.writeHead(502, { 'content-type': 'application/json' });
        res.end(JSON.stringify({ error: 'downstream failure', status: r.status }));
        return;
      }
      log('info', 'checkout ok');
      res.writeHead(200, { 'content-type': 'application/json' });
      res.end(JSON.stringify({ ok: true, downstream: JSON.parse(r.body) }));
    } catch (err) {
      log('error', 'checkout errored', { err: String(err) });
      res.writeHead(500, { 'content-type': 'application/json' });
      res.end(JSON.stringify({ error: String(err) }));
    }
    return;
  }

  res.writeHead(404).end('not found');
}).listen(PORT, () => log('info', `frontend listening on ${PORT}`));
