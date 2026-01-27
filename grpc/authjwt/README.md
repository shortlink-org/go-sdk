# authjwt

JWT validation package for gRPC services with JWKS support.

## Features

- RS256 signature validation with JWKS
- JWKS caching with configurable TTL
- Key refresh on cache miss (with thundering herd protection)
- Issuer and audience validation
- Clock skew tolerance (30s default)
- Prometheus metrics
- OpenTelemetry tracing

## Usage

### Create Validator

```go
import "github.com/shortlink-org/go-sdk/grpc/authjwt"

validator, err := authjwt.NewValidator(authjwt.ValidatorConfig{
    JWKSURL:         "https://oathkeeper.auth.svc/.well-known/jwks.json",
    Issuer:          "https://shortlink.best",
    Audience:        "shortlink-api",
    JWKSCacheTTL:    time.Hour,
    JWKSHTTPTimeout: 10 * time.Second,
    Leeway:          30 * time.Second,
})
```

### gRPC Server with JWT Validation

```go
server := grpc.NewServer(
    grpc.ChainUnaryInterceptor(
        authjwt.UnaryServerInterceptor(validator, authjwt.InterceptorConfig{
            SkipMethods: []string{"/myservice.Public/"},
        }),
    ),
    grpc.ChainStreamInterceptor(
        authjwt.StreamServerInterceptor(validator, authjwt.InterceptorConfig{}),
    ),
)
```

### Access Claims in Handler

```go
func (s *Server) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.User, error) {
    // Get validated claims
    claims := authjwt.ClaimsFromContext(ctx)
    if claims == nil {
        return nil, status.Error(codes.Unauthenticated, "not authenticated")
    }
    
    // Use claims
    userID := claims.Subject
    email := claims.Email
    
    // Or use helpers
    if !authjwt.IsAuthenticated(ctx) {
        return nil, status.Error(codes.Unauthenticated, "not authenticated")
    }
    
    subject := authjwt.GetSubject(ctx)
    // ...
}
```

## Claims Structure

The `Claims` struct extends `jwt.RegisteredClaims` with Oathkeeper-specific fields:

```go
type Claims struct {
    jwt.RegisteredClaims
    
    Email      string         `json:"email,omitempty"`
    Name       string         `json:"name,omitempty"`
    IdentityID string         `json:"identity_id,omitempty"`
    SessionID  string         `json:"session_id,omitempty"`
    Metadata   map[string]any `json:"metadata,omitempty"`
}
```

## Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `grpc_jwt_validations_total` | Counter | outcome, method | JWT validation attempts |
| `grpc_jwt_validation_seconds` | Histogram | outcome | Validation duration |

## Security Considerations

1. **HTTPS for JWKS** - always use HTTPS in production
2. **Short TTL** - use 15-minute token TTL
3. **Validate audience** - prevents token confusion attacks
4. **Validate issuer** - ensures token is from expected issuer
5. **JWKS cache** - 1 hour default, prevents excessive fetches
6. **Clock skew** - 30 second tolerance for distributed systems
