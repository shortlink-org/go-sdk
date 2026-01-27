# JWT Middleware

HTTP middleware for validating JWT tokens from Oathkeeper's `id_token` mutator.

## Architecture

```
Browser → Istio → Oathkeeper → BFF (this middleware) → Backend Services
                      ↓
              Validates session
              with Kratos, then
              issues JWT token
```

## Usage

```go
import (
    jwt_middleware "github.com/shortlink-org/go-sdk/http/middleware/jwt"
    "github.com/shortlink-org/go-sdk/auth/session"
)

// In your HTTP server setup
r.Use(jwt_middleware.JWT(cfg))

// In your handlers, access claims:
func MyHandler(w http.ResponseWriter, r *http.Request) {
    claims, err := session.GetClaims(r.Context())
    if err != nil {
        // Handle error
    }
    
    userID := claims.Subject
    email := claims.Email
    // ...
}
```

## Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `AUTH_LOGIN_URL` | `/auth/login` | URL to redirect unauthenticated browser requests |

## JWT Claims

The middleware expects JWT tokens with these claims (set by Oathkeeper):

```json
{
  "sub": "user-id",
  "email": "user@example.com",
  "name": "User Name",
  "identity_id": "kratos-identity-id",
  "session_id": "kratos-session-id",
  "metadata": {},
  "iss": "https://shortlink.best",
  "iat": 1234567890,
  "exp": 1234567890
}
```

## Trace Propagation

The middleware automatically extracts trace context from incoming headers:
- `traceparent` (W3C TraceContext)
- `b3` / `x-b3-*` (Zipkin/Istio)
- `baggage` (W3C Baggage)

This ensures trace continuity across the service mesh.

## Security Notes

- **Signature verification is skipped** — we trust Oathkeeper
- Tokens are validated by Oathkeeper before reaching BFF
- Only internal cluster traffic should reach this middleware
