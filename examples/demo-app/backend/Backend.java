// End of the demo chain: frontend (Node) -> api (Python) -> backend (Java).
// Uses the JDK's built-in HTTP server so the image needs no build tool and no
// dependencies. The OpenTelemetry Java agent instruments com.sun.net.httpserver.
import com.sun.net.httpserver.HttpExchange;
import com.sun.net.httpserver.HttpServer;

import java.io.IOException;
import java.io.OutputStream;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.util.concurrent.Executors;

public class Backend {

    private static final int PORT = Integer.parseInt(System.getenv().getOrDefault("PORT", "8080"));

    /**
     * Pulls the trace id out of the incoming W3C traceparent header, whose format
     * is version-traceid-spanid-flags. The agent propagates this automatically, so
     * reading the header correlates a log line with its trace without pulling in a
     * logging framework or the OpenTelemetry API.
     */
    private static String traceIdOf(HttpExchange exchange) {
        String traceparent = exchange.getRequestHeaders().getFirst("traceparent");
        if (traceparent == null) {
            return null;
        }
        String[] parts = traceparent.split("-");
        if (parts.length < 3 || parts[1].length() != 32 || parts[1].chars().allMatch(c -> c == '0')) {
            return null;
        }
        return parts[1];
    }

    private static void log(String level, String msg, String traceId) {
        StringBuilder line = new StringBuilder()
                .append("{\"level\":\"").append(level)
                .append("\",\"msg\":\"").append(msg)
                .append("\",\"service\":\"backend\"");
        if (traceId != null) {
            line.append(",\"trace_id\":\"").append(traceId).append("\"");
        }
        System.out.println(line.append("}"));
    }

    private static void respond(HttpExchange exchange, int status, String body) throws IOException {
        byte[] bytes = body.getBytes(StandardCharsets.UTF_8);
        exchange.getResponseHeaders().add("content-type", "application/json");
        exchange.sendResponseHeaders(status, bytes.length);
        try (OutputStream out = exchange.getResponseBody()) {
            out.write(bytes);
        }
    }

    public static void main(String[] args) throws IOException {
        HttpServer server = HttpServer.create(new InetSocketAddress(PORT), 0);

        server.createContext("/healthz", ex -> respond(ex, 200, "{\"ok\":true}"));

        server.createContext("/inventory", ex -> {
            String query = ex.getRequestURI().getQuery();
            boolean fail = query != null && query.contains("fail=1");
            String traceId = traceIdOf(ex);

            if (fail) {
                // The deliberate error path the e2e suite looks for.
                log("error", "inventory lookup failed", traceId);
                respond(ex, 500, "{\"error\":\"inventory unavailable\"}");
                return;
            }

            log("info", "inventory checked", traceId);
            respond(ex, 200, "{\"sku\":\"ABC-123\",\"inStock\":true}");
        });

        server.setExecutor(Executors.newFixedThreadPool(4));
        log("info", "backend listening on " + PORT, null);
        server.start();
    }
}
