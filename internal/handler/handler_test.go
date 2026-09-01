package handler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/nubitel/dialplan-manager-stats/internal/model"
)

type fakeService struct {
	domain    string
	campaigns []string
}

func (f *fakeService) QueueMembers(_ context.Context, domain string, campaigns []string) (*model.QueueMemberStatsResponse, error) {
	f.domain, f.campaigns = domain, campaigns
	return &model.QueueMemberStatsResponse{DomainName: domain, CampaignIDs: campaigns, Records: []model.LiveQueueMember{}}, nil
}
func (f *fakeService) Agents(_ context.Context, domain string) (*model.AgentDashboardListResponse, error) {
	f.domain = domain
	return &model.AgentDashboardListResponse{Agents: []model.AgentDashboardResponse{}}, nil
}

func TestQueueMembersPreservesGetBodyContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &fakeService{}
	handler := New(service, slog.New(slog.NewTextHandler(io.Discard, nil)))
	router := gin.New()
	router.GET("/queues/queue-members-stats", handler.QueueMembers)
	request := httptest.NewRequest(http.MethodGet, "/queues/queue-members-stats?domain_name=example.com", strings.NewReader(`{"campaign_ids":["1001@example.com"]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || service.domain != "example.com" || len(service.campaigns) != 1 {
		t.Fatalf("unexpected response=%d body=%s domain=%s campaigns=%v", response.Code, response.Body, service.domain, service.campaigns)
	}
}

func TestAgentsRequiresDomain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := New(&fakeService{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	router := gin.New()
	router.GET("/agents", handler.Agents)
	request := httptest.NewRequest(http.MethodGet, "/agents", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
}
