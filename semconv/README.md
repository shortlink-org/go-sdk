# Semantic conventions

The telemetry a go-sdk module emits is part of its public API: dashboards and
alerts in every consuming service key on the metric names and attribute keys.
This directory is the single description of that telemetry, in the
[OpenTelemetry semantic convention format](https://github.com/open-telemetry/weaver/blob/main/schemas/semconv-syntax.md),
and the source the Go code is generated from with
[OpenTelemetry Weaver](https://github.com/open-telemetry/weaver).

```
semconv/
  registry/
    manifest.yaml        registry name, schema URL (version), OTel dependency
    common.yaml          attributes shared by more than one module
    watermill.yaml       attributes and metrics of the watermill module
    postgres.yaml        attributes and metrics of the read-replica router
    cqrs.yaml            attributes and metrics of the cqrs outbox forwarder
    eventsourcing.yaml   attributes and metrics of the event store
  templates/registry/go/ Weaver templates that render the registry as Go
  Makefile               check / generate / diff, all through the Weaver image
```

Generated code is committed next to the module that emits the signals, so a
consumer of the SDK never needs Weaver:

| Registry file             | Generated package                                                              |
|---------------------------|--------------------------------------------------------------------------------|
| `registry/watermill.yaml` | [`watermill/semconv`](../watermill/semconv)                                     |
| `registry/postgres.yaml`  | [`db/drivers/postgres/replica/semconv`](../db/drivers/postgres/replica/semconv) |
| `registry/cqrs.yaml`      | [`cqrs/semconv`](../cqrs/semconv)                                               |
| `registry/eventsourcing.yaml` | [`eventsourcing/semconv`](../eventsourcing/semconv)                         |

The module list is `MODULES` in the Makefile: module name, the prefix stripped
from the Go identifiers, and the output directory.

## Workflow

Change the registry, regenerate, commit both:

```bash
make -C semconv check
make -C semconv generate
```

CI runs `check`, regenerates and fails when the committed code differs from
the registry, and prints `weaver registry diff` against `main` in the job
summary so that a renamed or removed signal is visible in the pull request.

## Conventions

- One YAML file per module, one `registry.<module>` attribute group that
  declares every attribute the module records, and one `metric.<module>.*`
  group per instrument that `ref`s those attributes.
- Metric names are the names the module has always exported. Renaming one is a
  breaking change for every dashboard that queries it; the registry makes that
  visible, it does not make it free.
- `stability: development` until a signal has been queried in production for a
  release cycle. Deprecate with `deprecated:` instead of deleting, and keep
  the deprecated group until the next schema version.
- Prefer an existing OpenTelemetry attribute over a new one: `ref` it, as
  `postgres.yaml` does with `server.address` and `error.type`. The upstream
  registry is a dependency in `registry/manifest.yaml`; keep its version equal
  to the `go.opentelemetry.io/otel/semconv` import used by the modules. Only
  referenced upstream attributes reach the generated code.
- An attribute is defined once per registry. One that two modules record goes
  to `common.yaml` and is `ref`erenced from both.
- An empty `unit: ""` creates the instrument without a unit. It is deliberate
  where the Prometheus translation would otherwise append a suffix nobody
  expects: `postgres_replica_up` with unit `1` would become
  `postgres_replica_up_ratio`.

## Generated API

For each metric the Go template emits a struct wrapping the OpenTelemetry
instrument, a constructor that tolerates a nil meter, and an `Add` or `Record`
method whose positional parameters are the attributes the registry marks
`required`:

```go
published, err := semconv.NewMessagesPublishedTotal(meter)
published.Add(ctx, 1, topic, semconv.TraceID(traceID))

failed.Add(ctx, 1, semconv.StagePublish, topic, semconv.Error(err.Error()))
```

Attributes become `XKey` constants, `X(val)` constructors and, for enums, a
typed `XAttr` with one constant per member.
