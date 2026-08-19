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

// Consistency is what the database driver provides.
//
//	router, _ := postgres.RouterFrom(store)
//	port := router.Port()
type Consistency interface {
	// Watermark returns a token describing the writes made on ctx, and whether
	// there were any. A context that did not write costs nothing.
	Watermark(ctx context.Context) (string, bool, error)

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

// attach returns ctx with the watermark set on its outgoing metadata.
//
// Set rather than append: a token accumulated across several hops would leave
// the receiver picking one at random.
func attach(ctx context.Context, token string) context.Context {
	outgoing, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		outgoing = metadata.MD{}
	}

	updated := outgoing.Copy()
	updated.Set(MetadataKey, token)

	return metadata.NewOutgoingContext(ctx, updated)
}
