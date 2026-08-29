// Package grpcmw carries the driver's read-your-writes guarantee across a gRPC
// hop.
//
// The problem it closes: a service that writes and then calls another service
// hands over nothing, so the callee's reads may be served by a replica that has
// not replayed those writes yet. A context does not cross a wire, so the
// guarantee has to travel as metadata.
//
// Both directions are covered, and they are not the same case:
//
//   - Forward. The caller wrote, then called. Its watermark rides out in
//     metadata, and the callee's reads are gated on it.
//   - Return. The callee wrote on the caller's behalf. Its watermark comes back
//     in the trailer, and the caller's own later reads are gated on it.
//
// It lives with the driver, next to httpmw, because a boundary middleware for
// this guarantee is the driver's concern whichever transport it speaks. The
// dependency is only google.golang.org/grpc/metadata for the wire format;
// Consistency is declared here and satisfied by *postgres.TextPort, so the
// package still does not reach into the router's types.
package grpcmw

import (
	"context"
	"strings"

	"google.golang.org/grpc/metadata"
)

// MetadataKey carries the watermark, spelled in lower case because that is how
// gRPC normalizes metadata keys and therefore how it arrives.
const MetadataKey = "wal-watermark"

// unresolvedToken is intentionally unparsable by the database adapter. It
// carries one bit of information across a failed capture: a write happened,
// so the receiver must stay on the primary even though no LSN is available.
const unresolvedToken = "unresolved"

// Consistency is what the database driver provides.
//
//	router, _ := postgres.RouterFrom(store)
//	port := router.Port()
type Consistency interface {
	// Capture returns a token describing writes made on ctx. Empty means no
	// write; an error means a write happened but its position is unresolved.
	Capture(ctx context.Context) (string, error)

	// Apply returns a context carrying the guarantee a received token
	// describes.
	Apply(ctx context.Context, token string) context.Context

	// Observe raises the watermark of the tracker already in ctx, for a token
	// that came back from a callee.
	Observe(ctx context.Context, token string)

	// Scope returns a context whose reads may use a replica. A request that
	// arrives carrying no watermark still has to be scoped, or every one of
	// its reads defaults to the primary.
	Scope(ctx context.Context) context.Context
}

// skipMethodPrefixes are the infrastructure RPCs that never touch the
// database, matching what the session interceptors skip.
var skipMethodPrefixes = []string{
	"/grpc.health.v1.Health/",
	"/grpc.reflection.v1.ServerReflection/",
	"/grpc.reflection.v1alpha.ServerReflection/",
}

func shouldSkipMethod(fullMethod string) bool {
	for _, prefix := range skipMethodPrefixes {
		if strings.HasPrefix(fullMethod, prefix) {
			return true
		}
	}

	return false
}

// tokenFrom reads the watermark out of a metadata set.
func tokenFrom(md metadata.MD) string {
	values := md.Get(MetadataKey)
	if len(values) == 0 {
		return ""
	}

	return strings.TrimSpace(values[0])
}

type capturedWatermarkState uint8

const (
	capturedWatermarkAbsent capturedWatermarkState = iota
	capturedWatermarkResolved
	capturedWatermarkUnresolved
)

type capturedWatermark struct {
	state capturedWatermarkState
	token string
}

// capture converts the structural port's compatibility triple into one domain
// value. From here on, "nothing written", "resolved" and "unresolved" cannot
// be represented by contradictory booleans.
func capture(ctx context.Context, guarantee Consistency) capturedWatermark {
	token, err := guarantee.Capture(ctx)
	if err != nil {
		return capturedWatermark{state: capturedWatermarkUnresolved}
	}

	if token == "" {
		return capturedWatermark{state: capturedWatermarkAbsent}
	}

	return capturedWatermark{state: capturedWatermarkResolved, token: token}
}

func (w capturedWatermark) present() bool { return w.state != capturedWatermarkAbsent }

func (w capturedWatermark) wireValue() string {
	if w.state == capturedWatermarkResolved {
		return w.token
	}

	if w.state == capturedWatermarkUnresolved {
		return unresolvedToken
	}

	return ""
}

// attach returns ctx with the watermark set on its outgoing metadata. Set
// rather than append: several hops must not leave the receiver choosing a
// token at random.
func (w capturedWatermark) attach(ctx context.Context) context.Context {
	if !w.present() {
		return ctx
	}

	outgoing, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		outgoing = metadata.MD{}
	}

	updated := outgoing.Copy()
	updated.Set(MetadataKey, w.wireValue())

	return metadata.NewOutgoingContext(ctx, updated)
}

func (w capturedWatermark) setTrailer(set func(metadata.MD)) {
	if w.present() {
		set(metadata.Pairs(MetadataKey, w.wireValue()))
	}
}
