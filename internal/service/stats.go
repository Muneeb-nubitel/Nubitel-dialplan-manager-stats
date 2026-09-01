package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nubitel/dialplan-manager-stats/internal/metrics"
	"github.com/nubitel/dialplan-manager-stats/internal/model"
	redisrepo "github.com/nubitel/dialplan-manager-stats/internal/repository/redis"
	"golang.org/x/sync/singleflight"
)

type cacheEntry struct {
	value     any
	expiresAt time.Time
}

type Stats struct {
	repository redisrepo.Repository
	cacheTTL   time.Duration
	metrics    *metrics.Metrics
	cacheMu    sync.RWMutex
	cache      map[string]cacheEntry
	flights    singleflight.Group
}

func New(repository redisrepo.Repository, cacheTTL time.Duration, appMetrics *metrics.Metrics) *Stats {
	return &Stats{repository: repository, cacheTTL: cacheTTL, metrics: appMetrics, cache: make(map[string]cacheEntry)}
}

func (s *Stats) QueueMembers(ctx context.Context, domain string, campaignIDs []string) (*model.QueueMemberStatsResponse, error) {
	cleanCampaigns := cleanCampaignIDs(campaignIDs)
	if len(cleanCampaigns) == 0 {
		return nil, fmt.Errorf("at least one campaign is required")
	}

	recordsByCampaign := make(map[string][]model.LiveQueueMember, len(cleanCampaigns))
	missing := make([]string, 0, len(cleanCampaigns))
	for _, campaign := range cleanCampaigns {
		if value, ok := s.cacheGet(queueCacheKey(domain, campaign)); ok {
			s.metrics.Cache("queue_members", "hit")
			recordsByCampaign[campaign] = value.([]model.LiveQueueMember)
			continue
		}
		s.metrics.Cache("queue_members", "miss")
		missing = append(missing, campaign)
	}

	if len(missing) > 0 {
		loaded, err := s.loadQueueCampaigns(ctx, domain, missing)
		if err != nil {
			return nil, err
		}
		for campaign, records := range loaded {
			recordsByCampaign[campaign] = records
		}
	}

	records := make([]model.LiveQueueMember, 0)
	seen := make(map[string]struct{})
	for _, campaign := range cleanCampaigns {
		for _, member := range recordsByCampaign[campaign] {
			if _, ok := seen[member.CallUUID]; ok {
				continue
			}
			seen[member.CallUUID] = struct{}{}
			records = append(records, member)
		}
	}
	sortQueueMembers(records)
	return &model.QueueMemberStatsResponse{DomainName: domain, CampaignIDs: cleanCampaigns, Total: len(records), Records: records}, nil
}

func (s *Stats) loadQueueCampaigns(ctx context.Context, domain string, campaigns []string) (map[string][]model.LiveQueueMember, error) {
	batchCampaigns := append([]string(nil), campaigns...)
	sort.Strings(batchCampaigns)
	flightKey := "queue_batch|" + domain + "|" + strings.Join(batchCampaigns, ",")
	result := s.flights.DoChan(flightKey, func() (any, error) {
		loaded := make(map[string][]model.LiveQueueMember, len(batchCampaigns))
		stillMissing := make([]string, 0, len(batchCampaigns))
		for _, campaign := range batchCampaigns {
			if value, ok := s.cacheGet(queueCacheKey(domain, campaign)); ok {
				loaded[campaign] = value.([]model.LiveQueueMember)
			} else {
				loaded[campaign] = []model.LiveQueueMember{}
				stillMissing = append(stillMissing, campaign)
			}
		}
		if len(stillMissing) == 0 {
			return loaded, nil
		}

		rawRecords, err := s.repository.QueueMemberValues(ctx, domain, stillMissing)
		if err != nil {
			return nil, err
		}
		wanted := make(map[string]struct{}, len(stillMissing))
		for _, campaign := range stillMissing {
			wanted[campaign] = struct{}{}
		}
		activeStates := map[string]struct{}{"Waiting": {}, "Trying": {}, "Receiving": {}, "Answered": {}, "In a queue call": {}}
		for _, raw := range rawRecords {
			var member model.LiveQueueMember
			if err := json.Unmarshal(raw, &member); err != nil {
				return nil, fmt.Errorf("decode queue member: %w", err)
			}
			if member.Domain != domain {
				continue
			}
			if _, ok := wanted[member.Queue]; !ok {
				continue
			}
			if _, ok := activeStates[member.State]; !ok {
				continue
			}
			loaded[member.Queue] = append(loaded[member.Queue], member)
		}

		expiresAt := time.Now().Add(s.cacheTTL)
		s.cacheMu.Lock()
		for _, campaign := range stillMissing {
			s.cache[queueCacheKey(domain, campaign)] = cacheEntry{value: loaded[campaign], expiresAt: expiresAt}
		}
		s.cacheMu.Unlock()
		return loaded, nil
	})

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case completed := <-result:
		if completed.Err != nil {
			return nil, completed.Err
		}
		return completed.Val.(map[string][]model.LiveQueueMember), nil
	}
}

func queueCacheKey(domain, campaign string) string {
	return "queue|" + domain + "|" + campaign
}

func sortQueueMembers(records []model.LiveQueueMember) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].Priority != records[j].Priority {
			return records[i].Priority > records[j].Priority
		}
		return records[i].JoinedEpoch < records[j].JoinedEpoch
	})
}

func (s *Stats) Agents(ctx context.Context, domain string) (*model.AgentDashboardListResponse, error) {
	key := "agents|" + domain
	value, err := s.cached(ctx, "agents", key, func(loadCtx context.Context) (any, error) {
		values, err := s.repository.AgentValues(loadCtx, domain)
		if err != nil {
			return nil, err
		}
		agents := make([]model.AgentDashboardResponse, 0, len(values))
		for uuid, raw := range values {
			var agent model.AgentDashboard
			if err := json.Unmarshal([]byte(raw), &agent); err != nil {
				continue
			}
			if agent.UUID == "" {
				agent.UUID = uuid
			}
			agents = append(agents, model.AgentDashboardResponse{UUID: agent.UUID, AgentID: agent.AgentID, Domain: domain, Name: agent.Name, Status: agent.Status, State: agentState(agent.CallStatus)})
		}
		sort.Slice(agents, func(i, j int) bool { return agents[i].UUID < agents[j].UUID })
		return &model.AgentDashboardListResponse{Agents: agents}, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(*model.AgentDashboardListResponse), nil
}

func (s *Stats) cached(ctx context.Context, resource, key string, load func(context.Context) (any, error)) (any, error) {
	if value, ok := s.cacheGet(key); ok {
		s.metrics.Cache(resource, "hit")
		return value, nil
	}
	s.metrics.Cache(resource, "miss")
	result := s.flights.DoChan(key, func() (any, error) {
		if value, ok := s.cacheGet(key); ok {
			return value, nil
		}
		value, err := load(ctx)
		if err == nil {
			s.cacheMu.Lock()
			s.cache[key] = cacheEntry{value: value, expiresAt: time.Now().Add(s.cacheTTL)}
			s.cacheMu.Unlock()
		}
		return value, err
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case completed := <-result:
		return completed.Val, completed.Err
	}
}

func (s *Stats) cacheGet(key string) (any, bool) {
	s.cacheMu.RLock()
	entry, ok := s.cache[key]
	s.cacheMu.RUnlock()
	return entry.value, ok && time.Now().Before(entry.expiresAt)
}

func cleanCampaignIDs(campaignIDs []string) []string {
	clean := make([]string, 0, len(campaignIDs))
	seen := make(map[string]struct{}, len(campaignIDs))
	for _, campaign := range campaignIDs {
		campaign = strings.TrimSpace(campaign)
		if campaign == "" {
			continue
		}
		if _, ok := seen[campaign]; ok {
			continue
		}
		seen[campaign] = struct{}{}
		clean = append(clean, campaign)
	}
	return clean
}

func agentState(callStatus string) string {
	switch callStatus {
	case "Ringing":
		return "Receiving"
	case "Busy":
		return "In a Call"
	default:
		return "Waiting"
	}
}
