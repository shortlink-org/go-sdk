//go:build unit

package grpcmw

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// fakeConsistency records what the interceptors asked of it. The real
// implementation is *postgres.TextPort; the contract is exercised through the
// same structural interface a caller would satisfy, which keeps the test off
// the router and its pools.
type fakeConsistency struct {
	token     string
	hasToken  bool
	err       error
	applied   []string
	observed  []string
	scopeHits int
}

type marker struct{ name string }

func (f *fakeConsistency) Watermark(context.Context) (string, bool, error) {
	return f.token, f.hasToken, f.err
}

func (f *fakeConsistency) Apply(ctx context.Context, token string) context.Context {
	f.applied = append(f.applied, token)

	return context.WithValue(ctx, marker{"applied"}, token)
}

func (f *fakeConsistency) Observe(_ context.Context, token string) {
	f.observed = append(f.observed, token)
}

func (f *fakeConsistency) Scope(ctx context.Context) context.Context {
	f.scopeHits++

	return context.WithValue(ctx, marker{"scoped"}, true)
}

const testToken = "v1:42:1:16/B374D848:1755561600123"

func unaryInfo(method string) *grpc.UnaryServerInfo {
	return &grpc.UnaryServerInfo{FullMethod: method}
}

func TestClientAttachesTheWatermark(t *testing.T) {
	t.Parallel()

	fake := &fakeConsistency{token: testToken, hasToken: true}

	var sent metadata.MD

	err := UnaryClientInterceptor(fake)(context.Background(), "/svc/Method", nil, nil, nil,
		func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			sent, _ = metadata.FromOutgoingContext(ctx)

			return nil
		})

	require.NoError(t, err)
	assert.Equal(t, []string{testToken}, sent.Get(MetadataKey))
}

func TestClientSendsNothingWhenTheContextDidNotWrite(t *testing.T) {
	t.Parallel()

	fake := &fakeConsistency{hasToken: false}

	var sent metadata.MD

	err := UnaryClientInterceptor(fake)(context.Background(), "/svc/Method", nil, nil, nil,
		func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
			sent, _ = metadata.FromOutgoingContext(ctx)

			return nil
		})

	require.NoError(t, err)
	assert.Empty(t, sent.Get(MetadataKey))
}

// A watermark that could not be read must not fail the call: this is an
// optimization, and trading it for availability is the wrong way round.
func TestClientSwallowsAWatermarkError(t *testing.T) {
	t.Parallel()

	fake := &fakeConsistency{err: errors.New("primary unreachable")}
	called := false

	err := UnaryClientInterceptor(fake)(context.Background(), "/svc/Method", nil, nil, nil,
		func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			called = true

			return nil
		})

	require.NoError(t, err)
	assert.True(t, called)
}

func TestClientObservesTheReturnedWatermark(t *testing.T) {
	t.Parallel()

	fake := &fakeConsistency{}

	err := UnaryClientInterceptor(fake)(context.Background(), "/svc/Method", nil, nil, nil,
		func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, opts ...grpc.CallOption) error {
			fillTrailer(opts, metadata.Pairs(MetadataKey, testToken))

			return nil
		})

	require.NoError(t, err)
	assert.Equal(t, []string{testToken}, fake.observed)
}

// A call can fail after the write it made has committed. Forgetting that write
// is the case this whole mechanism exists to prevent.
func TestClientObservesTheWatermarkEvenWhenTheCallFails(t *testing.T) {
	t.Parallel()

	fake := &fakeConsistency{}
	sentinel := errors.New("unavailable")

	err := UnaryClientInterceptor(fake)(context.Background(), "/svc/Method", nil, nil, nil,
		func(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, opts ...grpc.CallOption) error {
			fillTrailer(opts, metadata.Pairs(MetadataKey, testToken))

			return sentinel
		})

	require.ErrorIs(t, err, sentinel)
	assert.Equal(t, []string{testToken}, fake.observed)
}

func TestServerAppliesAnIncomingWatermark(t *testing.T) {
	t.Parallel()

	fake := &fakeConsistency{}
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(MetadataKey, testToken))

	var handlerCtx context.Context

	_, err := UnaryServerInterceptor(fake)(ctx, nil, unaryInfo("/svc/Method"),
		//nolint:fatcontext // capturing the handler's context is the assertion
		func(handled context.Context, _ any) (any, error) {
			handlerCtx = handled

			return nil, nil
		})

	require.NoError(t, err)
	assert.Equal(t, []string{testToken}, fake.applied)
	assert.Equal(t, testToken, handlerCtx.Value(marker{"applied"}))
	assert.Zero(t, fake.scopeHits)
}

// A request arriving without a watermark still has to be scoped, or every one
// of its reads falls back to the primary and the replica stays idle.
func TestServerScopesARequestWithoutAWatermark(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		md   metadata.MD
	}{
		{name: "no metadata at all"},
		{name: "metadata without the key", md: metadata.Pairs("other", "x")},
		{name: "an empty value", md: metadata.Pairs(MetadataKey, "   ")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := incomingContext(t, tt.md)

			fake := &fakeConsistency{}

			var handlerCtx context.Context

			_, err := UnaryServerInterceptor(fake)(ctx, nil, unaryInfo("/svc/Method"),
				//nolint:fatcontext // capturing the handler's context is the assertion
				func(handled context.Context, _ any) (any, error) {
					handlerCtx = handled

					return nil, nil
				})

			require.NoError(t, err)
			assert.Equal(t, 1, fake.scopeHits)
			assert.Empty(t, fake.applied)
			assert.Equal(t, true, handlerCtx.Value(marker{"scoped"}))
		})
	}
}

func TestServerSkipsInfrastructureMethods(t *testing.T) {
	t.Parallel()

	for _, method := range []string{
		"/grpc.health.v1.Health/Check",
		"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo",
		"/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo",
	} {
		fake := &fakeConsistency{}

		_, err := UnaryServerInterceptor(fake)(context.Background(), nil, unaryInfo(method),
			func(context.Context, any) (any, error) { return nil, nil })

		require.NoError(t, err)
		assert.Zero(t, fake.scopeHits, "method: %s", method)
	}
}

func TestNilConsistencyIsATransparentPassThrough(t *testing.T) {
	t.Parallel()

	called := false

	_, err := UnaryServerInterceptor(nil)(context.Background(), nil, unaryInfo("/svc/Method"),
		func(context.Context, any) (any, error) {
			called = true

			return nil, nil
		})

	require.NoError(t, err)
	assert.True(t, called)

	called = false

	err = UnaryClientInterceptor(nil)(context.Background(), "/svc/Method", nil, nil, nil,
		func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			called = true

			return nil
		})

	require.NoError(t, err)
	assert.True(t, called)
}

func TestStreamClientAttachesTheWatermark(t *testing.T) {
	t.Parallel()

	fake := &fakeConsistency{token: testToken, hasToken: true}

	var sent metadata.MD

	_, err := StreamClientInterceptor(fake)(context.Background(), &grpc.StreamDesc{}, nil, "/svc/Stream",
		func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
			sent, _ = metadata.FromOutgoingContext(ctx)

			return nil, nil
		})

	require.NoError(t, err)
	assert.Equal(t, []string{testToken}, sent.Get(MetadataKey))
}

// attach must replace rather than append, or a token accumulated over several
// hops leaves the receiver picking one at random.
func TestAttachReplacesAnExistingToken(t *testing.T) {
	t.Parallel()

	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(MetadataKey, "stale"))

	updated, ok := metadata.FromOutgoingContext(attach(ctx, testToken))
	require.True(t, ok)
	assert.Equal(t, []string{testToken}, updated.Get(MetadataKey))
}

// incomingContext builds a server-side context carrying md, or none at all.
func incomingContext(t *testing.T, md metadata.MD) context.Context {
	t.Helper()

	if md == nil {
		return t.Context()
	}

	return metadata.NewIncomingContext(t.Context(), md)
}

// fillTrailer emulates what the transport does with grpc.Trailer call options.
func fillTrailer(opts []grpc.CallOption, md metadata.MD) {
	for _, opt := range opts {
		if trailer, ok := opt.(grpc.TrailerCallOption); ok {
			*trailer.TrailerAddr = md
		}
	}
}
