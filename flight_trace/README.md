# flight_trace

The `flight_trace` package provides a lightweight wrapper around the [Go runtime Flight Recorder](https://go.dev/blog/flight-recorder) (Go 1.25+),  
allowing you to programmatically control and dump runtime traces.

## 🧩 Features

- Initializes `runtime/trace.FlightRecorder` with graceful shutdown via `context.Context`
- Configuration through **Viper** (`FLIGHT_RECORDER_*` environment variables)
- On-demand dump creation (`DumpSnapshot`, `DumpSnapshotAsync`)
- Framework-agnostic — works in HTTP, gRPC, CLI, or background workers

## 🔌 Middleware

- HTTP: [`http/middleware/flight_trace`](../http/middleware/flight_trace)
- gRPC: [`grpc/middleware/flight_trace`](../grpc/middleware/flight_trace)

## ⚙️ Configuration

| Variable                    | Type     | Description                      | Default             |
|-----------------------------|----------|----------------------------------|---------------------|
| `FLIGHT_RECORDER_ENABLED`   | bool     | Enable Flight Recorder           | `true`              |
| `FLIGHT_RECORDER_MIN_AGE`   | duration | Minimum age of samples to retain | `1s`                |
| `FLIGHT_RECORDER_MAX_BYTES` | uint64   | Maximum buffer size in bytes     | `20971520` (20 MB)  |
| `FLIGHT_RECORDER_DUMP_PATH` | string   | Directory for dump files         | `/tmp/flight_dumps` |

## 📚 References

- [Go Blog: Flight Recorder](https://go.dev/blog/flight-recorder)
- [Last9: Trace Go Apps using runtime tracing & OpenTelemetry](https://last9.io/blog/trace-go-apps-using-runtime-tracing-and-opentelemetry/#why-go%E2%80%99s-runtime-tracing-outperforms-traditional-profiling)
- [Habr: Profiling Go applications with Flight Recorder (RU)](https://habr.com/ru/articles/951216/)
- [Habr: Go runtime tracing under the hood (RU)](https://habr.com/ru/articles/802107/)

---

Minimal dependencies. Maximum observability.  
Use `flight_trace` as a standalone utility or integrate it into your SDK.
