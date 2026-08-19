# Prometheus Operator rules

`read_replica_rules.yaml` is a `PrometheusRule` for the read-replica routing:
three recording rules and eight alerts.

```bash
kubectl apply -f read_replica_rules.yaml -n <your-namespace>
```

> The Operator only picks a rule up when its labels match the `ruleSelector` of
> your `Prometheus` resource. The manifest ships with
> `release: kube-prometheus-stack`; change it to yours. A rule nobody selects is
> a rule that silently never runs.

## Recording rules

| rule | what it is |
|---|---|
| `job:postgres_route_offload:ratio5m` | share of reads a replica served — the number the feature exists for |
| `job:postgres_route_decisions_primary:rate5m` | statements the primary still serves |
| `job_reason:postgres_route_fallbacks:rate5m` | reads that wanted a replica and got the primary, by reason |

The offload ceiling is arithmetic, not aspirational: every request reads, and a
request that also writes taints itself so its own read returns to the primary.
At a read share of `r` the most that can leave is `r/(2−r)` — 33% at a 50/50
mix, not 50%. Do not alert on it being below 50%.

## Alerts

| alert | severity | why it matters |
|---|---|---|
| `PostgresReplicaPromoted` | critical | a standby left recovery and was quarantined for the process lifetime. A topology change, not a blip — reconfigure and restart |
| `PostgresNoHealthyReplica` | warning | every read is falling back to the primary |
| `PostgresReplicaProbeFailing` | warning | the gate is open on guesswork. This is the design's most uncomfortable corner, and nothing else surfaces it |
| `PostgresReplicaLagHigh` | warning | the standby is not keeping up with writes |
| `PostgresRoutingNotWired` | info | replicas are healthy but reads arrive unscoped, so they default to the primary. The middleware is missing |
| `PostgresReplicaOffloadCollapsed` | info | replicas are eligible and almost nothing is routed to them |
| `PostgresWatermarkCaptureFailing` | warning | cross-boundary read-after-write is degrading silently |
| `PostgresConsistencyGateNotCatchingUp` | warning | messages keep arriving before their writes replayed; the wait budget is too small for the lag |

Two of these guard corners the ADR calls out as the ones that will page you.
A failed probe leaves the gate deciding on stale information, and a promoted
standby keeps answering queries while no longer being a replica of anything —
neither fails loudly on its own.

## Metric names

The names are what the OpenTelemetry Prometheus exporter publishes, which is not
what the instruments are called in Go. See
[../grafana/README.md](../grafana/README.md) for the suffix rules that make the
difference.

See [ADR 0001 — Read-replica routing](../../ADR/0001-read-replica-routing.md).
