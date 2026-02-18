// Package main is the entry point for the oficina server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/albertocavalcante/oficina/internal/api"
	"github.com/albertocavalcante/oficina/internal/store"
)

const (
	defaultPort       = 8080
	readHeaderTimeout = 30 * time.Second
	shutdownTimeout   = 10 * time.Second
)

func main() {
	var port int

	cmd := &cobra.Command{
		Use:   "oficina-server",
		Short: "Oficina control plane server",
		RunE: func(_ *cobra.Command, _ []string) error {
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelInfo,
			}))

			s := store.New()
			srv := api.New(s, logger)

			addr := fmt.Sprintf(":%d", port)
			logger.Info("starting oficina server",
				"addr", addr,
			)
			logger.Info("endpoints",
				"submit_job", fmt.Sprintf("POST http://localhost:%d/api/jobs", port),
				"list_jobs", fmt.Sprintf("GET  http://localhost:%d/api/jobs", port),
				"agents", fmt.Sprintf("GET  http://localhost:%d/api/agents", port),
				"dashboard", fmt.Sprintf("http://localhost:%d/", port),
			)

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()

			httpServer := &http.Server{
				Addr:              addr,
				Handler:           srv.Handler(),
				ReadHeaderTimeout: readHeaderTimeout,
			}

			// Start server in background.
			errCh := make(chan error, 1)
			go func() { errCh <- httpServer.ListenAndServe() }()

			// Wait for interrupt or server error.
			select {
			case <-ctx.Done():
				logger.Info("shutting down")
				shutCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
				defer cancel()
				if err := httpServer.Shutdown(shutCtx); err != nil {
					return fmt.Errorf("shutdown: %w", err)
				}
				return nil
			case err := <-errCh:
				if errors.Is(err, http.ErrServerClosed) {
					return nil
				}
				return err
			}
		},
	}

	cmd.Flags().IntVar(&port, "port", defaultPort, "listen port")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
