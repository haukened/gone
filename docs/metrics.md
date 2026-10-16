# Metrics

Metrics are off unless both `GONE_METRICS_ADDR` and `GONE_METRICS_TOKEN` are set. An address without a token logs a warning and leaves metrics off. The token should be at least 16 characters: a shorter one logs a warning at startup and a future release will refuse it. Generate one with `openssl rand -base64 24`.

They get their own listener on `GONE_METRICS_ADDR` (for example `127.0.0.1:9090`), separate from the app on `GONE_ADDR`, so the public site never serves metrics. Keep that port on loopback or a private network.

## The metrics listener

Every request must carry `Authorization: Bearer <token>`. The token is compared in constant time (as SHA-256 digests, so the comparison does not depend on its length).

| Request | Response |
| ------- | -------- |
| `GET /metrics` | Prometheus text exposition format |
| `GET /` | JSON snapshot of the counters and summaries |
| Missing or wrong token, on any path | `401`, checked before anything else |
| Another method on `/` or `/metrics` | `405` |
| Any other path | `404` |

Quick check:

```sh
curl -H 'Authorization: Bearer <token>' http://127.0.0.1:9090/metrics
```

## Prometheus

A scrape config, with the token in a file Prometheus can read:

```yaml
scrape_configs:
  - job_name: gone
    authorization:
      credentials_file: /etc/prometheus/gone_token
    static_configs:
      - targets: ['gone:9090']
```

Counters are stored in SQLite, so they survive restarts rather than resetting to zero. Every counter is listed from the first scrape, at `0` until it first increments.

| Name | Type | Meaning |
| ---- | ---- | ------- |
| `gone_secrets_created_total` | counter | Secrets stored |
| `gone_secrets_consumed_total` | counter | Secrets opened and deleted |
| `gone_secrets_expired_deleted_total` | counter | Secrets the janitor removed: expired ones, and opened ones whose claim lease lapsed |
| `gone_secrets_claims_expired_total` | counter | Opened secrets deleted because the recipient never confirmed receipt within the claim lease (a subset of `gone_secrets_expired_deleted_total`) |
| `gone_secrets_revoked_total` | counter | Secrets the sender deleted with the manage link before they were opened |
| `gone_rate_limited_create_total` | counter | Requests rejected with `429` by the create budget (`GONE_RATE_CREATE`) |
| `gone_rate_limited_read_total` | counter | Requests rejected with `429` by the read budget (`GONE_RATE_READ`) |
| `gone_requests_created_total` | counter | Secret requests opened |
| `gone_requests_filled_total` | counter | Secret requests answered |
| `gone_requests_opened_total` | counter | Request replies opened and deleted |
| `gone_requests_cancelled_total` | counter | Requests (or unopened replies) the requester cancelled |
| `gone_requests_expired_total` | counter | Requests the janitor removed because nobody answered in time |
| `gone_janitor_deleted_per_cycle` | summary | Expirations per janitor run (`_sum` and `_count`, with lifetime `_min` and `_max` as gauges) |
| `gone_secrets_stored` | gauge | Live secrets right now, including request replies |
| `gone_secrets_stored_bytes` | gauge | Total ciphertext size of those secrets |
| `gone_requests_open` | gauge | Secret requests waiting for a reply |
| `gone_build_info` | gauge | Always `1`, with labels `version` and `goversion` |
| `go_goroutines`, `go_memstats_heap_alloc_bytes`, `go_memstats_sys_bytes`, `go_gc_cycles_total`, `process_start_time_seconds` | gauge / counter | Go runtime and process basics, named as in the official Go client |

The storage gauges are counted when scraped and include only live rows: not expired, and not past a lapsed claim. No metric exposes an ID or ciphertext.

## JSON snapshot

`GET /` on the metrics listener returns the same counters and summaries, without the `gone_` prefix:

```json
{
	"counters": {
		"secrets_created_total": 123,
		"secrets_consumed_total": 118,
		"secrets_expired_deleted_total": 47
	},
	"summaries": {
		"janitor_deleted_per_cycle": {"count": 42, "sum": 420, "min": 1, "max": 25}
	}
}
```

## How the counters are kept

- Counts accumulate in memory and are flushed to SQLite periodically. A snapshot merges what was persisted with the deltas since the last flush.
- A graceful stop flushes one last time.
