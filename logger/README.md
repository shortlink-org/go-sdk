# Logger

An OpenTelemetry-aware `slog.Handler`, plus a thin constructor around it.

This module deliberately exposes **no logger interface of its own**. Everything
in go-sdk accepts the standard library's `*slog.Logger`, so if you already have
one, you do not need this module at all — the handler is the only part worth
importing.

## Quick Start

```go
import "github.com/shortlink-org/go-sdk/logger"

log, err := logger.New(logger.Configuration{Level: logger.INFO_LEVEL})
if err != nil {
    return err
}

log.Info("Server started", slog.Int("port", 8080))
log.Error("Connection failed", slog.Any("error", err))

// Pass a context to correlate the line with the active span
log.InfoContext(ctx, "Request processed", slog.String("method", "GET"))
```

To attach the handler to a logger you build yourself:

```go
handler, err := logger.NewHandler(logger.Default())
if err != nil {
    return err
}

log := slog.New(handler).With(slog.String("service", "shortlink"))
```

## API

```go
logger.New(Configuration) (*slog.Logger, error)   // handler + slog.New
logger.NewHandler(Configuration) (slog.Handler, error)
logger.Default() Configuration
```

Logging itself is the standard `*slog.Logger` API: `Debug/Info/Warn/Error` and
their `…Context` variants.

## Configuration

```go
type Configuration struct {
    Writer     io.Writer // default: os.Stdout
    TimeFormat string    // default: time.RFC3339Nano
    Level      int       // ERROR_LEVEL, WARN_LEVEL, INFO_LEVEL, DEBUG_LEVEL
}
```

## OpenTelemetry correlation

The handler enriches a record **only when the call carried a real context** —
that is, through `InfoContext`, `ErrorContext` and friends. The bare
`Info`/`Error` methods pass `context.Background()`, which the handler treats as
"no trace to correlate with".

Given a context with an active span, the handler adds a `log.<LEVEL>` event to
that span and stamps `traceID` / `spanID` onto the record. For WARN and ERROR
without an active span it opens a short correlation span instead.

## Dependencies

OpenTelemetry (`otel`, `otel/trace`), and nothing else from this repository.
