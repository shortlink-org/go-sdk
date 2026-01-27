# authforward

Package for capturing and forwarding JWT tokens across gRPC service boundaries.

## Overview

This package implements the "token relay" pattern:

```
HTTP Client → BFF → Link Service → Metadata Service
              │        │                │
              └────────┴────────────────┘
                   JWT forwarded automatically
```

## Usage

### BFF: Capture HTTP Authorization and forward to gRPC

```go
import (
    "github.com/shortlink-org/go-sdk/grpc/authforward"
    "google.golang.org/grpc/metadata"
)

func (h *Handler) CreateLink(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()
    
    // Capture Authorization header from HTTP request
    auth := r.Header.Get("Authorization")
    if auth != "" {
        ctx = authforward.WithToken(ctx, auth)
    }
    
    // gRPC call - token automatically forwarded by client interceptor
    resp, err := h.linkClient.CreateLink(ctx, req)
    // ...
}
```

### gRPC Server Setup (Link Service)

```go
import (
    "github.com/shortlink-org/go-sdk/grpc/authforward"
    "github.com/shortlink-org/go-sdk/grpc/authjwt"
)

func main() {
    // Create JWT validator
    validator, _ := authjwt.NewValidator(authjwt.ValidatorConfig{
        JWKSURL:  "https://oathkeeper.auth.svc/.well-known/jwks.json",
        Issuer:   "https://shortlink.best",
        Audience: "shortlink-api",
    })
    
    // Server with interceptors
    server := grpc.NewServer(
        grpc.ChainUnaryInterceptor(
            // 1. Validate JWT
            authjwt.UnaryServerInterceptor(validator, authjwt.InterceptorConfig{}),
            // 2. Capture token for forwarding to downstream services
            authforward.UnaryServerInterceptor(),
        ),
        grpc.ChainStreamInterceptor(
            authjwt.StreamServerInterceptor(validator, authjwt.InterceptorConfig{}),
            authforward.StreamServerInterceptor(),
        ),
    )
    
    // Register services...
}
```

### gRPC Client Setup (calling downstream services)

```go
import "github.com/shortlink-org/go-sdk/grpc/authforward"

func main() {
    conn, _ := grpc.Dial(
        "metadata-service:50051",
        grpc.WithChainUnaryInterceptor(
            authforward.UnaryClientInterceptor(),
        ),
        grpc.WithChainStreamInterceptor(
            authforward.StreamClientInterceptor(),
        ),
    )
    
    client := metadata.NewMetadataServiceClient(conn)
    
    // Token is automatically forwarded from context
    resp, err := client.GetMetadata(ctx, req)
}
```

## Security Considerations

1. **This package does NOT validate tokens** - use with `authjwt` for validation
2. **Use only within trusted mesh boundaries** - mTLS recommended
3. **Token TTL** - use short-lived tokens (15 min recommended)
4. **Audience validation** - always validate to prevent token confusion attacks
5. **No accumulation** - uses Set instead of Append to prevent header injection
