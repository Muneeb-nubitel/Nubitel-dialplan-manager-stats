# dialplan-manager-stats

Lightweight read service for the live call-center supervisor dashboard. It reads
the existing Dialplan Manager/IVR Flow Designer Redis structures and exposes:

- `GET /queues/queue-members-stats?domain_name=<domain>` with the existing JSON body `{ "campaign_ids": [...] }`
- `GET /agents?domain_name=<domain>`
- compatibility aliases below `/call-center/api/v1`
- `GET /health`, `GET /ready`, and `GET /metrics`

## Why this service exists

Dashboard requests are isolated from Dialplan Manager. Identical requests are
cached per campaign for seven seconds by default and concurrent identical cache
misses are coalesced, so overlapping requests reuse campaign data and only
missing campaigns are read from Redis.

## Configuration

| Variable | Default | Description |
|---|---:|---|
| `PORT` | `5012` | HTTP port (hard-coded; environment value is ignored) |
| `GIN_MODE` | `release` | Gin mode |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, or `error` |
| `REDIS_HOST` | required | Redis hostname |
| `REDIS_PORT` | `6379` | Redis port |
| `REDIS_PWD` | empty | Redis password; matches Dialplan Manager |
| `REQUEST_TIMEOUT` | `5s` | Per-request deadline |
| `CACHE_TTL` | `7s` | Snapshot cache lifetime |
| `HTTP_READ_TIMEOUT` | `30s` | HTTP read timeout |
| `HTTP_WRITE_TIMEOUT` | `60s` | HTTP write timeout |
| `HTTP_IDLE_TIMEOUT` | `120s` | HTTP keep-alive timeout |
| `SHUTDOWN_TIMEOUT` | `15s` | Graceful shutdown limit |

Redis database, key prefix, pool, retry, batch, and timeout settings are fixed
in code. Only `REDIS_HOST`, `REDIS_PORT`, and `REDIS_PWD` are configurable.

## Run

```bash
export REDIS_HOST=localhost
go run ./cmd/server
```

Example queue request (the GET body is retained for API compatibility):

```bash
curl -X GET 'http://localhost:8080/queues/queue-members-stats?domain_name=example.com' \
  -H 'Content-Type: application/json' \
  -d '{"campaign_ids":["1001@example.com"]}'
```

## Kubernetes probes

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
readinessProbe:
  httpGet:
    path: /ready
    port: 8080
```

## Logging

Logs are written to stdout and a file whose name contains the application start
timestamp and process ID, for example
`logs/dialplan-manager-stats-20260901-163000-pid1234.log`. The file is rotated
when it reaches 20 MB. Rotated files are gzip-compressed and retained for 10
days.

## Caching

Queue-member data is cached independently for each domain and campaign. When a
request contains both cached and missing campaigns, the service makes one
repository call containing all missing campaigns, caches each result separately
(including empty results), and combines only the requested campaigns in the
response. Agent data is cached independently per domain.

The service is intentionally Redis-only. Unlike the original Dialplan Manager,
it does not query PostgreSQL to distinguish an unknown domain from a valid
domain with no live data; both produce an empty `200` snapshot.

