package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Port              string
	GinMode           string
	LogLevel          string
	RedisHost         string
	RedisPort         string
	RedisPassword     string
	RedisDB           int
	RedisKeyPrefix    string
	RedisPoolSize     int
	RedisMinIdleConns int
	RedisDialTimeout  time.Duration
	RedisReadTimeout  time.Duration
	RedisWriteTimeout time.Duration
	RedisPoolTimeout  time.Duration
	RedisMaxRetries   int
	HTTPReadTimeout   time.Duration
	HTTPWriteTimeout  time.Duration
	HTTPIdleTimeout   time.Duration
	ShutdownTimeout   time.Duration
	RequestTimeout    time.Duration
	CacheTTL          time.Duration
	MGetBatchSize     int
}

func Load() (Config, error) {
	cfg := Config{
		Port:              "5012",
		GinMode:           env("GIN_MODE", "release"),
		LogLevel:          env("LOG_LEVEL", "info"),
		RedisHost:         strings.TrimSpace(os.Getenv("REDIS_HOST")),
		RedisPort:         env("REDIS_PORT", "6379"),
		RedisPassword:     os.Getenv("REDIS_PWD"),
		RedisKeyPrefix:    "DIALPLAN_BACKEND:",
		RedisDB:           0,
		RedisPoolSize:     10,
		RedisMinIdleConns: 3,
		RedisMaxRetries:   2,
		MGetBatchSize:     500,
		RedisDialTimeout:  10 * time.Second,
		RedisReadTimeout:  10 * time.Second,
		RedisWriteTimeout: 10 * time.Second,
		RedisPoolTimeout:  20 * time.Second,
		HTTPReadTimeout:   envDuration("HTTP_READ_TIMEOUT", 30*time.Second),
		HTTPWriteTimeout:  envDuration("HTTP_WRITE_TIMEOUT", 60*time.Second),
		HTTPIdleTimeout:   envDuration("HTTP_IDLE_TIMEOUT", 120*time.Second),
		ShutdownTimeout:   envDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
		RequestTimeout:    envDuration("REQUEST_TIMEOUT", 5*time.Second),
		CacheTTL:          envDuration("CACHE_TTL", 7*time.Second),
	}
	if cfg.RedisHost == "" {
		return Config{}, fmt.Errorf("REDIS_HOST is required")
	}
	if cfg.RedisPoolSize < 1 || cfg.RedisMinIdleConns < 0 || cfg.MGetBatchSize < 1 {
		return Config{}, fmt.Errorf("Redis pool and batch settings must be positive")
	}
	if cfg.RedisMinIdleConns > cfg.RedisPoolSize {
		return Config{}, fmt.Errorf("REDIS_MIN_IDLE_CONNS cannot exceed REDIS_POOL_SIZE")
	}
	if !strings.HasSuffix(cfg.RedisKeyPrefix, ":") && cfg.RedisKeyPrefix != "" {
		cfg.RedisKeyPrefix += ":"
	}
	return cfg, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
