package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nubitel/dialplan-manager-stats/internal/config"
	"github.com/nubitel/dialplan-manager-stats/internal/handler"
	"github.com/nubitel/dialplan-manager-stats/internal/metrics"
	"github.com/nubitel/dialplan-manager-stats/internal/middleware"
)

type readinessChecker interface {
	Ping(context.Context) error
}

func NewRouter(
	cfg config.Config,
	apiHandler *handler.Handler,
	appMetrics *metrics.Metrics,
	readiness readinessChecker,
	logger *slog.Logger,
) *gin.Engine {
	gin.SetMode(cfg.GinMode)

	router := gin.New()
	router.Use(
		middleware.RequestID(),
		middleware.Recovery(logger),
		appMetrics.Middleware(),
		middleware.RequestLogger(logger),
		middleware.Timeout(cfg.RequestTimeout),
	)

	router.GET("/health", health)
	router.GET("/ready", ready(readiness))
	router.GET("/metrics", appMetrics.Handler())
	registerStatsRoutes(router, apiHandler)

	return router
}

func health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func ready(readiness readinessChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if err := readiness.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	}
}

func registerStatsRoutes(router *gin.Engine, apiHandler *handler.Handler) {
	// Short routes are the standalone service contract. The existing full paths
	// are aliases so callers can migrate by changing only their upstream host.
	// router.GET("/queues/queue-members-stats", apiHandler.QueueMembers)
	// router.GET("/agents", apiHandler.Agents)
	router.GET("/call-center-stats/api/v1/queues/queue-members-stats", apiHandler.QueueMembers)
	router.GET("/call-center-stats/api/v1/agents", apiHandler.Agents)
}
