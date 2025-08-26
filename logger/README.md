# Logger

Simple logger for Go built on `log/slog`.

## Quick Start

```go
import "github.com/shortlink-org/go-sdk/logger"

// Create logger
cfg := logger.Configuration{Level: logger.INFO_LEVEL}
log, _ := logger.New(cfg)

// Basic logging
log.Info("Server started", "port", 8080)
log.Error("Connection failed", "error", err)

// With context
ctx := context.Background()
log.InfoWithContext(ctx, "Request processed", "method", "GET")

// With methods
userLogger := log.WithFields("user_id", "123")
userLogger.Info("User action", "action", "login")
```

## API

```go
// Core
logger.New(config) -> *SlogLogger

// Methods
log.Error(msg, fields...)
log.Warn(msg, fields...)
log.Info(msg, fields...)
log.Debug(msg, fields...)

// Context
log.ErrorWithContext(ctx, msg, fields...)
log.WarnWithContext(ctx, msg, fields...)
log.InfoWithContext(ctx, msg, fields...)
log.DebugWithContext(ctx, msg, fields...)

// With
log.WithFields(fields...) -> *SlogLogger
log.WithError(err) -> *SlogLogger
log.WithTags(map[string]string) -> *SlogLogger
```

## Configuration

```go
type Configuration struct {
    Writer     io.Writer // default: os.Stdout
    TimeFormat string    // default: time.RFC3339Nano
    Level      int       // ERROR_LEVEL, WARN_LEVEL, INFO_LEVEL, DEBUG_LEVEL
}
```

## Features

- JSON structured logging
- OpenTelemetry integration
- Context support
- High performance
- Zero external dependencies
