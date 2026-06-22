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

func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(Path, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

func Run(ctx context.Context, logger *slog.Logger) {
	server := &http.Server{
		Addr:              Address,
		Handler:           Handler(),
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
