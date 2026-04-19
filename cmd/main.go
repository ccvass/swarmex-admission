package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"os"
	"os/signal"
	"syscall"

	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/client"

	controller "github.com/ccvass/swarmex/swarmex-admission"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil { logger.Error("docker client failed", "error", err); os.Exit(1) }
	defer cli.Close()

	ctrl := controller.New(cli, "/etc/swarmex/admission.yaml", logger)

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
		logger.Info("health endpoint", "addr", ":8080")
		http.ListenAndServe(":8080", nil)
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	logger.Info("swarmex-admission starting")

	msgCh, errCh := cli.Events(ctx, events.ListOptions{})
	for {
		select {
		case event := <-msgCh:
			ctrl.HandleEvent(ctx, event)
		case err := <-errCh:
			if ctx.Err() != nil { logger.Info("shutdown complete"); return }
			logger.Error("event stream error", "error", err); return
		case <-ctx.Done():
			logger.Info("shutdown complete"); return
		}
	}
}
