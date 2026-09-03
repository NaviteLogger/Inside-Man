// Command bff serves the Inside Man API: the three signals plus Kubernetes
// topology, normalised around service identity.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NaviteLogger/Inside-Man/bff/internal/api"
	"github.com/NaviteLogger/Inside-Man/bff/internal/config"
	"github.com/NaviteLogger/Inside-Man/bff/internal/kube"
	"github.com/NaviteLogger/Inside-Man/bff/internal/promql"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	prom, err := promql.New(cfg.PrometheusURL, cfg.Window)
	if err != nil {
		return err
	}

	// Blocks until the informer caches sync, so the first request served is
	// already answered from a warm cache.
	log.Info("syncing kubernetes informers")
	cache, err := kube.NewCache(ctx, 10*time.Minute)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.NewServer(cfg, prom, cache, log).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			log.Error("shutdown", "err", err)
		}
	}()

	log.Info("listening", "addr", cfg.Addr, "prometheus", cfg.PrometheusURL)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
