# Grafana

`read_replica_dashboard.json` — import it, pick a Prometheus datasource, and it
answers three questions in order: is the routing paying off, can the replicas be
trusted, and what is the guarantee costing.

## Rows

**Is it paying off?** The share of reads a replica served, what the primary is
still doing, worst lag, and how many replicas are eligible. The offload figure
is the one to watch, and its ceiling is arithmetic rather than aspirational: a
request that writes taints itself, so its own reads return to the primary. At a
read share of `r` the most that can ever leave is `r/(2−r)` — 33% at a 50/50
mix, not 50%.

**Routing decisions.** Every statement by target and reason, and the diagnosis
panel next to it: reads that wanted a replica and got the primary. A climbing
`no_tracker` there means the feature is enabled but not wired — nothing is
scoping requests, so they all default to the primary.

**Replica health.** Lag in bytes, eligibility, probe failures and promotions,
probe duration.

**Crossing a boundary.** Time spent waiting for a replica to catch up, and the
queue gate's outcomes.

## Worth alerting on

| condition | why |
|---|---|
| `rate(postgres_replica_probe_failures_total[5m]) > 0` for 5m | the gate opens on a failed probe. Reads may be going stale with nothing else to show for it — this is the design's most uncomfortable corner |
| `increase(postgres_replica_promotions_total[10m]) > 0` | a standby left recovery and was quarantined for the process lifetime. A topology change, not a blip |
| `sum(postgres_replica_up) == 0` for 5m | every read is falling back to the primary |
| `max(postgres_replica_lag_bytes)` over your budget | the standby is not keeping up with writes |
| sustained `watermill_consistency_gate_results_total{outcome="not_caught_up"}` | the wait budget is too small for the lag, and those messages are going to the retry middleware |

## Metric names

The names are what the OpenTelemetry Prometheus exporter actually publishes, not
what the instruments are called in Go — the two differ. The exporter appends
`_total` to counters and a unit suffix to everything, de-duplicating when the
name already carries one. Two consequences worth knowing before editing a query:

- A gauge declared with unit `"1"` comes out as `..._ratio`. `postgres_replica_up`
  is therefore declared without a unit.
- Histograms arrive as `_bucket`, `_sum` and `_count` series, so a quantile needs
  `_bucket` and a rate needs `_count`.

The `server.address` attribute becomes the `server_address` label.

See [ADR 0001 — Read-replica routing](../../ADR/0001-read-replica-routing.md), and
the alerting rules in [../prometheus/](../prometheus/).
