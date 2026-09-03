// Package config reads the BFF's settings from the environment. The chart is
// the only thing that sets these, so defaults target a local cluster.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Addr string

	PrometheusURL string
	LokiURL       string
	TempoURL      string

	// Used only to build deep links for the browser, so this has to be an
	// address the user can reach. A cluster-internal service name will not do.
	GrafanaURL string

	// Namespaces the BFF reports on. Empty means every namespace it can read.
	Namespaces []string

	// The namespace Inside Man itself runs in. Log queries exclude it, because
	// the platform logs the very trace id being asked about: nginx records it
	// in the request URL and Loki records it in the query it just ran.
	SelfNamespace string

	// Health thresholds, mirroring design doc 5.2.
	ErrorRateWarn float64
	ErrorRateCrit float64
	P95Warn       time.Duration

	// Window used for every rate() and histogram_quantile query.
	Window time.Duration

	QueryTimeout time.Duration
}

func Load() (Config, error) {
	c := Config{
		Addr:          env("BFF_ADDR", ":8080"),
		PrometheusURL: env("PROMETHEUS_URL", "http://inside-man-prometheus.inside-man.svc:80"),
		LokiURL:       env("LOKI_URL", "http://inside-man-loki-gateway.inside-man.svc:80"),
		TempoURL:      env("TEMPO_URL", "http://inside-man-tempo.inside-man.svc:3200"),
		GrafanaURL:    env("GRAFANA_EXTERNAL_URL", "http://localhost:3000"),
		SelfNamespace: env("POD_NAMESPACE", "inside-man"),
	}

	var err error
	if c.ErrorRateWarn, err = envFloat("HEALTH_ERROR_RATE_WARN", 0.01); err != nil {
		return c, err
	}
	if c.ErrorRateCrit, err = envFloat("HEALTH_ERROR_RATE_CRIT", 0.05); err != nil {
		return c, err
	}
	if c.P95Warn, err = envDuration("HEALTH_P95_WARN", 500*time.Millisecond); err != nil {
		return c, err
	}
	if c.Window, err = envDuration("QUERY_WINDOW", 5*time.Minute); err != nil {
		return c, err
	}
	if c.QueryTimeout, err = envDuration("QUERY_TIMEOUT", 10*time.Second); err != nil {
		return c, err
	}

	if c.ErrorRateWarn > c.ErrorRateCrit {
		return c, fmt.Errorf("HEALTH_ERROR_RATE_WARN (%v) exceeds HEALTH_ERROR_RATE_CRIT (%v)",
			c.ErrorRateWarn, c.ErrorRateCrit)
	}
	return c, nil
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envFloat(key string, def float64) (float64, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return f, nil
}

func envDuration(key string, def time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return d, nil
}
