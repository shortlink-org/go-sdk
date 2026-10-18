## HTTP client/server

### Middleware

| Name                                      | Description                                            |
|-------------------------------------------|--------------------------------------------------------|
| [Auth](./middleware/auth)                 | This middleware authenticates the request.             |
| [Logger](./middleware/logger)             | This middleware logs the request.                      |
| [Metrics](./middleware/metrics)           | This middleware creates a new prometheus metrics.      |
| [Pprof Labels](./middleware/pprof_labels) | This middleware adds route labels to pprof.            |
| [RequestSize](./middleware/request_size)  | This middleware limits the request size.               |
| [SingleFlight](./middleware/singleflight) | This middleware shares the response.                   |
| [Span](./middleware/span)                 | Completes the otelhttp server span: `{method} {http.route}` name, `http.route`, `error.type` on 5xx; sets `trace_id` on the response header. |
