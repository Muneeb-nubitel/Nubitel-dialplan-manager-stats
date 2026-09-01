package redis

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	goredis "github.com/go-redis/redis/v8"
	"github.com/nubitel/dialplan-manager-stats/internal/metrics"
)

func TestRepositoryReadsIndexedMembersAndAgents(t *testing.T) {
	server := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	repository := &RedisRepository{
		client:    client,
		prefix:    "DIALPLAN_BACKEND:",
		mgetBatch: 1,
		metrics:   metrics.New(),
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	t.Cleanup(func() { _ = repository.Close() })

	ctx := context.Background()
	memberKey := "DIALPLAN_BACKEND:live:queue_members:example.com:1001@example.com:call-1"
	indexKey := "DIALPLAN_BACKEND:live:queue_member_index:example.com:1001@example.com"
	if err := client.Set(ctx, memberKey, `{"callUuid":"call-1"}`, time.Hour).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.ZAdd(ctx, indexKey, &goredis.Z{Score: float64(time.Now().Add(time.Hour).Unix()), Member: memberKey}).Err(); err != nil {
		t.Fatal(err)
	}
	if err := client.HSet(ctx, "DIALPLAN_BACKEND:agents_call_status:example.com:", "agent-1", `{"uuid":"agent-1"}`).Err(); err != nil {
		t.Fatal(err)
	}

	values, err := repository.QueueMemberValues(ctx, "example.com", []string{"1001@example.com"})
	if err != nil || len(values) != 1 || string(values[0]) != `{"callUuid":"call-1"}` {
		t.Fatalf("unexpected queue values=%q err=%v", values, err)
	}
	agents, err := repository.AgentValues(ctx, "example.com")
	if err != nil || agents["agent-1"] != `{"uuid":"agent-1"}` {
		t.Fatalf("unexpected agents=%v err=%v", agents, err)
	}
}
