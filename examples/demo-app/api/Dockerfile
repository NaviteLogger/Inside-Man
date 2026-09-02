# Debian base, chosen over Alpine. The OpenTelemetry Python agent ships
# glibc-compiled native wheels, and psutil fails to relocate against musl with
# "symbol not found: mallinfo", which aborts the instrumentation bootstrap.
FROM python:3.13-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY server.py .
EXPOSE 8080
USER nobody
CMD ["python", "-u", "server.py"]
