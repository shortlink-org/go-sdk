# Kratos Package

This Go package provides a client for interacting with [Ory Kratos](https://www.ory.sh/kratos/) Admin API.

## Overview

The Kratos package wraps the Ory Kratos Admin API client to provide a simple interface for retrieving user information, specifically user email by user ID. This is commonly used for private link access control and other authorization scenarios.

## Usage

### Creating a Client

```go
import (
    "github.com/shortlink-org/go-sdk/kratos"
    "github.com/shortlink-org/go-sdk/config"
    "github.com/shortlink-org/go-sdk/logger"
)

// Initialize client
kratosClient, err := kratos.New(log, cfg)
if err != nil {
    log.Fatalf("Failed to create Kratos client: %v", err)
}
```

### Getting User Email

```go
email, err := kratosClient.GetUserEmail(ctx, userID)
if err != nil {
    // Handle error (user not found, email missing, etc.)
    return err
}
// Use email for access control
```

## Configuration

The client reads configuration from environment variables:

| Name               | Description          | Default Value        |
| ------------------ | -------------------- | -------------------- |
| `KRATOS_ADMIN_URL` | Kratos Admin API URL | `http://kratos:4434` |

## Error Handling

All errors are logged and returned to the caller. According to security best practices (ADR-42), errors should be treated as permission denied to avoid revealing information about user existence.

## Requirements

- go 1.25.5 or higher
- Ory Kratos Admin API must be accessible
- Valid Kratos identity with email in traits
