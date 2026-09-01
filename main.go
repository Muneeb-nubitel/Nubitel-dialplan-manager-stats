package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/nubitel/dialplan-manager-stats/internal/config"
	"github.com/nubitel/dialplan-manager-stats/internal/handler"
	"github.com/nubitel/dialplan-manager-stats/internal/logging"
	"github.com/nubitel/dialplan-manager-stats/internal/metrics"
	redisrepo "github.com/nubitel/dialplan-manager-stats/internal/repository/redis"
	"github.com/nubitel/dialplan-manager-stats/internal/service"
	"github.com/nubitel/dialplan-manager-stats/server"
)

func main() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		slog.Error("Loading .env failed", "error", err)
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("Configuration failed", "error", err)
		os.Exit(1)
	}
	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)
	logger.Info("Application starting", "app", "dialplan-manager-stats", "port", cfg.Port)

	appMetrics := metrics.New()
	repository := redisrepo.New(cfg, logger, appMetrics)
	defer repository.Close()
	startupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := repository.Ping(startupCtx); err != nil {
		cancel()
		logger.Error("Redis connection failed", "error", err)
		os.Exit(1)
	}
	cancel()
	logger.Info("Redis connection successful", "host", cfg.RedisHost, "port", cfg.RedisPort, "database", cfg.RedisDB)

	statsService := service.New(repository, cfg.CacheTTL, appMetrics)
	apiHandler := handler.New(statsService, logger)
	router := server.NewRouter(cfg, apiHandler, appMetrics, repository, logger)
	server := &http.Server{Addr: ":" + cfg.Port, Handler: router, ReadTimeout: cfg.HTTPReadTimeout, WriteTimeout: cfg.HTTPWriteTimeout, IdleTimeout: cfg.HTTPIdleTimeout, ReadHeaderTimeout: 5 * time.Second}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("HTTP server starting", "address", server.Addr)
		serverErrors <- server.ListenAndServe()
	}()

	signalCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	select {
	case <-signalCtx.Done():
		logger.Info("Graceful shutdown starting")
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
		return
	}
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Graceful shutdown failed", "error", err)
		return
	}
	logger.Info("Graceful shutdown completed")
}

func newLogger(level string) *slog.Logger {
	var parsed slog.Level
	switch strings.ToLower(level) {
	case "debug":
		parsed = slog.LevelDebug
	case "warn", "warning":
		parsed = slog.LevelWarn
	case "error":
		parsed = slog.LevelError
	default:
		parsed = slog.LevelInfo
	}
	return logging.New(parsed)
}
