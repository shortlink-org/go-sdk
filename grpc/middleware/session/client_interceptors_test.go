package sessioninterceptor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/shortlink-org/go-sdk/auth/session"
	"github.com/shortlink-org/go-sdk/grpc/authforward"
)

const testAuthHeader = "Bearer token-1"

func TestWithAuthorizationRoundTrip(t *testing.T) {
	t.Run("stores and reads back the header", func(t *testing.T) {
		ctx := WithAuthorization(context.Background(), testAuthHeader)

		require.Equal(t, testAuthHeader, GetAuthorization(ctx))
	})

	t.Run("also feeds authforward", func(t *testing.T) {
		ctx := WithAuthorization(context.Background(), testAuthHeader)

		require.Equal(t, testAuthHeader, authforward.TokenFromContext(ctx))
	})

	t.Run("an empty header is stored but not forwarded", func(t *testing.T) {
		ctx := WithAuthorization(context.Background(), "")

		require.Empty(t, GetAuthorization(ctx))
		require.Empty(t, authforward.TokenFromContext(ctx))
	})

	t.Run("a context that was never populated reads empty", func(t *testing.T) {
		require.Empty(t, GetAuthorization(context.Background()))
	})
}

func TestAttachAuthMetadata(t *testing.T) {
	t.Run("attaches the header and the user id", func(t *testing.T) {
		ctx := WithAuthorization(context.Background(), testAuthHeader)
		ctx = session.WithUserID(ctx, testUserID)

		out, err := attachAuthMetadata(ctx)
		require.NoError(t, err)

		md, ok := metadata.FromOutgoingContext(out)
		require.True(t, ok)
		require.Equal(t, []string{testAuthHeader}, md.Get(authorizationKey))
		require.Equal(t, []string{testUserID}, md.Get(userIDKey))
	})

	t.Run("with nothing to attach the context is left alone", func(t *testing.T) {
		out, err := attachAuthMetadata(context.Background())
		require.NoError(t, err)

		_, ok := metadata.FromOutgoingContext(out)
		require.False(t, ok, "no outgoing metadata should have been created")
	})

	t.Run("only the user id", func(t *testing.T) {
		out, err := attachAuthMetadata(session.WithUserID(context.Background(), testUserID))
		require.NoError(t, err)

		md, _ := metadata.FromOutgoingContext(out)
		require.Empty(t, md.Get(authorizationKey))
		require.Equal(t, []string{testUserID}, md.Get(userIDKey))
	})

	t.Run("existing unrelated metadata survives", func(t *testing.T) {
		ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("trace-id", "abc"))
		ctx = WithAuthorization(ctx, testAuthHeader)

		out, err := attachAuthMetadata(ctx)
		require.NoError(t, err)

		md, _ := metadata.FromOutgoingContext(out)
		require.Equal(t, []string{"abc"}, md.Get("trace-id"))
		require.Equal(t, []string{testAuthHeader}, md.Get(authorizationKey))
	})
}

// Each hop replaces the header rather than appending, so a call that crosses
// several services does not accumulate duplicates.
func TestAttachAuthMetadataReplacesRatherThanAppends(t *testing.T) {
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs(authorizationKey, "Bearer stale"))
	ctx = WithAuthorization(ctx, testAuthHeader)

	out, err := attachAuthMetadata(ctx)
	require.NoError(t, err)

	md, _ := metadata.FromOutgoingContext(out)
	require.Equal(t, []string{testAuthHeader}, md.Get(authorizationKey))
}

func TestSessionUnaryClientInterceptorAttachesMetadata(t *testing.T) {
	interceptor := SessionUnaryClientInterceptor()

	ctx := session.WithUserID(WithAuthorization(context.Background(), testAuthHeader), testUserID)

	var seen metadata.MD

	invoker := func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		seen, _ = metadata.FromOutgoingContext(ctx)

		return nil
	}

	require.NoError(t, interceptor(ctx, testMethod, nil, nil, nil, invoker))
	require.Equal(t, []string{testAuthHeader}, seen.Get(authorizationKey))
	require.Equal(t, []string{testUserID}, seen.Get(userIDKey))
}

func TestSessionStreamClientInterceptorAttachesMetadata(t *testing.T) {
	interceptor := SessionStreamClientInterceptor()

	ctx := WithAuthorization(context.Background(), testAuthHeader)

	var seen metadata.MD

	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		seen, _ = metadata.FromOutgoingContext(ctx)

		return nil, nil
	}

	_, err := interceptor(ctx, &grpc.StreamDesc{}, nil, testMethod, streamer)

	require.NoError(t, err)
	require.Equal(t, []string{testAuthHeader}, seen.Get(authorizationKey))
}
