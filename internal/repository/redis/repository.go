package redis

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	goredis "github.com/go-redis/redis/v8"
	"github.com/nubitel/dialplan-manager-stats/internal/config"
	"github.com/nubitel/dialplan-manager-stats/internal/metrics"
)

const (
	queueMemberIndexPattern = "live:queue_member_index:%s:%s"
	agentDashboardPattern   = "agents_call_status:%s:"
)

type Repository interface {
	Ping(context.Context) error
	Close() error
	QueueMemberValues(context.Context, string, []string) ([][]byte, error)
	AgentValues(context.Context, string) (map[string]string, error)
}

type RedisRepository struct {
	client    *goredis.Client
	prefix    string
	mgetBatch int
	metrics   *metrics.Metrics
	logger    *slog.Logger
}

func New(cfg config.Config, logger *slog.Logger, appMetrics *metrics.Metrics) *RedisRepository {
	client := goredis.NewClient(&goredis.Options{
		Addr:         cfg.RedisHost + ":" + cfg.RedisPort,
		Password:     cfg.RedisPassword,
		DB:           cfg.RedisDB,
		MaxRetries:   cfg.RedisMaxRetries,
		DialTimeout:  cfg.RedisDialTimeout,
		ReadTimeout:  cfg.RedisReadTimeout,
		WriteTimeout: cfg.RedisWriteTimeout,
		PoolSize:     cfg.RedisPoolSize,
		MinIdleConns: cfg.RedisMinIdleConns,
		PoolTimeout:  cfg.RedisPoolTimeout,
	})
	return &RedisRepository{client: client, prefix: cfg.RedisKeyPrefix, mgetBatch: cfg.MGetBatchSize, metrics: appMetrics, logger: logger}
}

func (r *RedisRepository) Ping(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		r.recordError("ping", err)
		return err
	}
	r.logger.Debug("Redis ping completed")
	return nil
}

func (r *RedisRepository) Close() error { return r.client.Close() }

func (r *RedisRepository) key(key string) string {
	if r.prefix == "" || strings.HasPrefix(key, r.prefix) {
		return key
	}
	return r.prefix + key
}

func (r *RedisRepository) QueueMemberValues(ctx context.Context, domain string, campaigns []string) ([][]byte, error) {
	r.logger.Debug("Redis queue member read started", "domain", domain, "campaign_count", len(campaigns))
	allKeys := make([]string, 0)
	now := strconv.FormatInt(unixNow(), 10)
	for _, campaign := range campaigns {
		indexKey := r.key(fmt.Sprintf(queueMemberIndexPattern, domain, campaign))
		pipe := r.client.Pipeline()
		pipe.ZRemRangeByScore(ctx, indexKey, "-inf", now)
		active := pipe.ZRangeByScore(ctx, indexKey, &goredis.ZRangeBy{Min: "(" + now, Max: "+inf"})
		if _, err := pipe.Exec(ctx); err != nil {
			r.recordError("queue_index", err)
			return nil, fmt.Errorf("read queue member index: %w", err)
		}
		keys, err := active.Result()
		if err != nil {
			r.recordError("queue_index", err)
			return nil, fmt.Errorf("read queue member index: %w", err)
		}
		allKeys = append(allKeys, keys...)
	}

	values := make([][]byte, 0, len(allKeys))
	for start := 0; start < len(allKeys); start += r.mgetBatch {
		end := start + r.mgetBatch
		if end > len(allKeys) {
			end = len(allKeys)
		}
		result, err := r.client.MGet(ctx, allKeys[start:end]...).Result()
		if err != nil {
			r.recordError("queue_mget", err)
			return nil, fmt.Errorf("bulk read queue members: %w", err)
		}
		for _, raw := range result {
			if value, ok := raw.(string); ok {
				values = append(values, []byte(value))
			}
		}
	}
	r.logger.Debug("Redis queue member read completed", "domain", domain, "key_count", len(allKeys), "record_count", len(values))
	return values, nil
}

func (r *RedisRepository) AgentValues(ctx context.Context, domain string) (map[string]string, error) {
	r.logger.Debug("Redis agent read started", "domain", domain)
	values, err := r.client.HGetAll(ctx, r.key(fmt.Sprintf(agentDashboardPattern, domain))).Result()
	if err != nil {
		r.recordError("agents_hgetall", err)
		return nil, fmt.Errorf("read dashboard agents: %w", err)
	}
	r.logger.Debug("Redis agent read completed", "domain", domain, "agent_count", len(values))
	return values, nil
}

func (r *RedisRepository) recordError(operation string, err error) {
	r.metrics.RedisError(operation)
	r.logger.Error("Redis operation failed", "operation", operation, "error", err)
}

var unixNow = func() int64 { return timeNow().Unix() }
var timeNow = time.Now
