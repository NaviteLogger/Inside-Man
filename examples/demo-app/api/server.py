"""Middle of the demo chain: frontend (Node) -> api (Python) -> backend (Java).

Flask and requests are used because the OpenTelemetry Python agent ships
instrumentation for both. The standard library's http.server has none, so a
stdlib service produces no spans and silently breaks the trace chain.
"""
import json
import logging
import os

import requests
from flask import Flask, jsonify, request

BACKEND = os.environ.get("BACKEND_URL", "http://demo-backend:8080")
PORT = int(os.environ.get("PORT", "8080"))

app = Flask(__name__)
logging.getLogger("werkzeug").setLevel(logging.ERROR)


class JsonFormatter(logging.Formatter):
    """Emits JSON, carrying whatever trace context the agent attached.

    With OTEL_PYTHON_LOG_CORRELATION=true the agent adds otelTraceID and
    otelSpanID to every LogRecord, which is what makes "logs for this trace"
    work without the application knowing anything about tracing.
    """

    def format(self, record):
        payload = {
            "level": record.levelname.lower(),
            "msg": record.getMessage(),
            "service": "api",
        }
        trace_id = getattr(record, "otelTraceID", None)
        if trace_id and trace_id != "0" * 32:
            payload["trace_id"] = trace_id
            payload["span_id"] = getattr(record, "otelSpanID", "")
        extra = getattr(record, "extra_fields", None)
        if extra:
            payload.update(extra)
        return json.dumps(payload)


_handler = logging.StreamHandler()
_handler.setFormatter(JsonFormatter())
log_ = logging.getLogger("api")
log_.setLevel(logging.INFO)
log_.addHandler(_handler)
log_.propagate = False


def log(level, msg, **extra):
    getattr(log_, level)(msg, extra={"extra_fields": extra} if extra else None)


@app.get("/healthz")
def healthz():
    return jsonify(ok=True)


@app.get("/order")
def order():
    fail = request.args.get("fail") == "1"
    path = "/inventory?fail=1" if fail else "/inventory"
    try:
        resp = requests.get(f"{BACKEND}{path}", timeout=5)
        if resp.status_code >= 500:
            log("error", "backend returned an error", status=resp.status_code)
            return jsonify(error="backend failure", status=resp.status_code), 502
        log("info", "order placed")
        return jsonify(order="placed", inventory=resp.json())
    except Exception as exc:  # noqa: BLE001
        log("error", "backend call failed", err=str(exc))
        return jsonify(error=str(exc)), 500


if __name__ == "__main__":
    log("info", f"api listening on {PORT}")
    app.run(host="0.0.0.0", port=PORT, threaded=True)
