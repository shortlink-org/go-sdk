package http_client

import "github.com/shortlink-org/go-sdk/http/client/internal/types"

// Metrics is an alias for types.Metrics for backward compatibility.
type Metrics = types.Metrics

// NewMetrics creates a new Metrics instance.
func NewMetrics(namespace, subsystem string) *Metrics {
	return types.NewMetrics(namespace, subsystem)
}

// Label constants for backward compatibility.
const (
	LabelClient = types.LabelClient
	LabelHost   = types.LabelHost
	LabelMethod = types.LabelMethod
	LabelSource = types.LabelSource
)
