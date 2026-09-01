package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nubitel/dialplan-manager-stats/internal/metrics"
)

type fakeRepository struct {
	mu          sync.Mutex
	queueCalls  int
	agentCalls  int
	queueValues [][]byte
	queueByID   map[string][][]byte
	queueLoads  [][]string
	agentValues map[string]string
}

func (f *fakeRepository) Ping(context.Context) error { return nil }
func (f *fakeRepository) Close() error               { return nil }
func (f *fakeRepository) QueueMemberValues(_ context.Context, _ string, campaigns []string) ([][]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queueCalls++
	f.queueLoads = append(f.queueLoads, append([]string(nil), campaigns...))
	if f.queueByID != nil {
		values := make([][]byte, 0)
		for _, campaign := range campaigns {
			values = append(values, f.queueByID[campaign]...)
		}
		return values, nil
	}
	return f.queueValues, nil
}
func (f *fakeRepository) AgentValues(context.Context, string) (map[string]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agentCalls++
	return f.agentValues, nil
}

func TestQueueMembersFiltersSortsAndCaches(t *testing.T) {
	repository := &fakeRepository{queueValues: [][]byte{
		[]byte(`{"callUuid":"low","queue":"1001@example.com","domain":"example.com","joined_epoch":100,"priority":1,"state":"Waiting"}`),
		[]byte(`{"callUuid":"high","queue":"1001@example.com","domain":"example.com","joined_epoch":200,"priority":5,"state":"Receiving"}`),
		[]byte(`{"callUuid":"done","queue":"1001@example.com","domain":"example.com","priority":9,"state":"Finished"}`),
	}}
	stats := New(repository, time.Minute, metrics.New())

	for range 2 {
		result, err := stats.QueueMembers(context.Background(), "example.com", []string{"1001@example.com"})
		if err != nil {
			t.Fatal(err)
		}
		if result.Total != 2 || result.Records[0].CallUUID != "high" || result.Records[1].CallUUID != "low" {
			t.Fatalf("unexpected result: %+v", result)
		}
	}
	if repository.queueCalls != 1 {
		t.Fatalf("expected one cached repository read, got %d", repository.queueCalls)
	}
}

func TestQueueMembersCachesEachCampaignAndBatchesOnlyMissingCampaigns(t *testing.T) {
	repository := &fakeRepository{queueByID: map[string][][]byte{
		"1001@example.com": {[]byte(`{"callUuid":"call-1","queue":"1001@example.com","domain":"example.com","priority":1,"state":"Waiting"}`)},
		"1002@example.com": {[]byte(`{"callUuid":"call-2","queue":"1002@example.com","domain":"example.com","priority":2,"state":"Receiving"}`)},
		"1003@example.com": {[]byte(`{"callUuid":"call-3","queue":"1003@example.com","domain":"example.com","priority":3,"state":"Trying"}`)},
	}}
	stats := New(repository, time.Minute, metrics.New())

	first, err := stats.QueueMembers(context.Background(), "example.com", []string{"1001@example.com", "1002@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 2 || repository.queueCalls != 1 {
		t.Fatalf("expected one batched load with two records, got result=%+v calls=%d", first, repository.queueCalls)
	}
	if fmt.Sprint(repository.queueLoads[0]) != "[1001@example.com 1002@example.com]" {
		t.Fatalf("unexpected first batch: %v", repository.queueLoads[0])
	}

	second, err := stats.QueueMembers(context.Background(), "example.com", []string{"1002@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Total != 1 || second.Records[0].CallUUID != "call-2" || repository.queueCalls != 1 {
		t.Fatalf("expected campaign 1002 from cache, got result=%+v calls=%d", second, repository.queueCalls)
	}

	third, err := stats.QueueMembers(context.Background(), "example.com", []string{"1002@example.com", "1003@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if third.Total != 2 || repository.queueCalls != 2 {
		t.Fatalf("expected one additional load, got result=%+v calls=%d", third, repository.queueCalls)
	}
	if fmt.Sprint(repository.queueLoads[1]) != "[1003@example.com]" {
		t.Fatalf("expected only campaign 1003 to be loaded, got %v", repository.queueLoads[1])
	}
}

func TestQueueMembersCachesEmptyCampaign(t *testing.T) {
	repository := &fakeRepository{queueByID: map[string][][]byte{}}
	stats := New(repository, time.Minute, metrics.New())

	for range 2 {
		result, err := stats.QueueMembers(context.Background(), "example.com", []string{"empty@example.com"})
		if err != nil {
			t.Fatal(err)
		}
		if result.Total != 0 {
			t.Fatalf("expected empty result, got %+v", result)
		}
	}
	if repository.queueCalls != 1 {
		t.Fatalf("expected empty campaign to be cached, got %d repository calls", repository.queueCalls)
	}
}

func TestAgentsMapsStateAndSkipsInvalidRecords(t *testing.T) {
	repository := &fakeRepository{agentValues: map[string]string{
		"agent-b": `{"agent_id":2,"name":"B","status":"Available","call_status":"Busy"}`,
		"agent-a": `{"uuid":"agent-a","agent_id":1,"name":"A","status":"Available","call_status":"Ringing"}`,
		"broken":  `{`,
	}}
	stats := New(repository, time.Minute, metrics.New())
	result, err := stats.Agents(context.Background(), "example.com")
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(result.Agents[0].UUID, result.Agents[0].State) != "agent-aReceiving" {
		t.Fatalf("unexpected first agent: %+v", result.Agents[0])
	}
	if fmt.Sprint(result.Agents[1].UUID, result.Agents[1].State) != "agent-bIn a Call" {
		t.Fatalf("unexpected second agent: %+v", result.Agents[1])
	}
}
