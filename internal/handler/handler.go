package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/nubitel/dialplan-manager-stats/internal/model"
)

type Service interface {
	QueueMembers(context.Context, string, []string) (*model.QueueMemberStatsResponse, error)
	Agents(context.Context, string) (*model.AgentDashboardListResponse, error)
}

type Handler struct {
	service Service
	logger  *slog.Logger
}

func New(service Service, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

func (h *Handler) QueueMembers(c *gin.Context) {
	domain := strings.TrimSpace(c.Query("domain_name"))
	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain_name cannot be empty"})
		return
	}
	var request model.QueueMemberStatsRequest
	if err := c.ShouldBindJSON(&request); err != nil || len(request.CampaignIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "campaign_ids must be a non-empty list"})
		return
	}
	stats, err := h.service.QueueMembers(c.Request.Context(), domain, request.CampaignIDs)
	if err != nil {
		h.internalError(c, "queue member stats failed", err)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *Handler) Agents(c *gin.Context) {
	domain := strings.TrimSpace(c.Query("domain_name"))
	if domain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "domain_name cannot be empty"})
		return
	}
	agents, err := h.service.Agents(c.Request.Context(), domain)
	if err != nil {
		h.internalError(c, "list agents failed", err)
		return
	}
	c.JSON(http.StatusOK, agents)
}

func (h *Handler) internalError(c *gin.Context, message string, err error) {
	requestID, _ := c.Get("request_id")
	h.logger.Error("Unexpected handler error", "message", message, "request_id", requestID, "error", err)
	status := http.StatusInternalServerError
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		status = http.StatusServiceUnavailable
	}
	c.JSON(status, gin.H{"error": message})
}
