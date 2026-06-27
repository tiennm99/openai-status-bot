package health

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

const (
	Address = "127.0.0.1:8080"
	Path    = "/healthz"
)

// Check reports whether the bot is ready to serve. It returns nil when healthy
// and an error describing why not otherwise. A nil Check is always healthy.
type Check func(ctx context.Context) error

// Handler serves the health endpoint. It returns 200 only when check passes,
// and 503 while the bot is still starting or a dependency is unreachable, so an
// orchestrator probe reflects real readiness instead of a static 200.
func Handler(check Check) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(Path, func(w http.ResponseWriter, r *http.Request) {
		if check != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := check(ctx); err != nil {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte("unavailable\n"))
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

func Run(ctx context.Context, logger *slog.Logger, check Check) {
	server := &http.Server{
		Addr:              Address,
		Handler:           Handler(check),
		ReadHeaderTimeout: 3 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Warn("shutdown health server", "error", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Warn("health server stopped", "error", err)
	}
}
