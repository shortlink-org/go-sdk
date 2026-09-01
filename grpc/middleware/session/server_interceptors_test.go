package sessioninterceptor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/shortlink-org/go-sdk/auth/session"
)

const (
	testUserID = "user-42"
	testMethod = "/billing.v1.Billing/CreateOrder"
)

// noopSpan is what the interceptors get from a context with no tracer set up.
//
//nolint:ireturn // trace.Span is an interface in the OTel API.
func noopSpan() trace.Span {
	return trace.SpanFromContext(context.Background())
}

func incoming(pairs ...string) context.Context {
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(pairs...))
}

func TestShouldSkipMethod(t *testing.T) {
	tests := []struct {
		name   string
		method string
		want   bool
	}{
		{name: "health check", method: "/grpc.health.v1.Health/Check", want: true},
		{name: "reflection v1", method: "/grpc.reflection.v1.ServerReflection/ServerReflectionInfo", want: true},
		{name: "reflection v1alpha", method: "/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo", want: true},
		{name: "business method", method: testMethod, want: false},
		{name: "lookalike is not skipped", method: "/grpc.healthy.v1.Health/Check", want: false},
		{name: "empty", method: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldSkipMethod(tt.method))
		})
	}
}

func TestSplitFullMethodName(t *testing.T) {
	tests := []struct {
		name       string
		full       string
		wantSvc    string
		wantMethod string
	}{
		{name: "well formed", full: testMethod, wantSvc: "billing.v1.Billing", wantMethod: "CreateOrder"},
		{name: "missing leading slash", full: "billing.v1.Billing/CreateOrder", wantSvc: "", wantMethod: ""},
		{name: "too many parts", full: "/a/b/c", wantSvc: "", wantMethod: ""},
		{name: "empty", full: "", wantSvc: "", wantMethod: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, method := splitFullMethodName(tt.full)
			require.Equal(t, tt.wantSvc, svc)
			require.Equal(t, tt.wantMethod, method)
		})
	}
}

func TestClassifyAuthError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantCode   grpcCodes.Code
		wantReason string
	}{
		{
			name:       "missing metadata",
			err:        ErrServerMissingMetadata,
			wantCode:   grpcCodes.Unauthenticated,
			wantReason: "missing_metadata",
		},
		{
			name:       "missing user id",
			err:        ErrServerMissingUserID,
			wantCode:   grpcCodes.Unauthenticated,
			wantReason: "missing_user_id",
		},
		{
			name:       "wrapped sentinel still classifies",
			err:        errors.Join(errors.New("outer"), ErrServerMissingUserID),
			wantCode:   grpcCodes.Unauthenticated,
			wantReason: "missing_user_id",
		},
		{
			name:       "anything else is internal",
			err:        errors.New("boom"),
			wantCode:   grpcCodes.Internal,
			wantReason: "internal_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, reason := classifyAuthError(tt.err)
			require.Equal(t, tt.wantCode, code)
			require.Equal(t, tt.wantReason, reason)
		})
	}
}

func TestFirstNonEmptyMetadataValue(t *testing.T) {
	md := metadata.Pairs(
		xUserIDKey, "   ",
		userIDKey, "from-user-id",
		xUserEmailKey, "  spaced@example.com  ",
	)

	t.Run("skips a blank value and moves to the next key", func(t *testing.T) {
		require.Equal(t, "from-user-id", firstNonEmptyMetadataValue(md, xUserIDKey, userIDKey))
	})

	t.Run("trims surrounding whitespace", func(t *testing.T) {
		require.Equal(t, "spaced@example.com", firstNonEmptyMetadataValue(md, xUserEmailKey))
	})

	t.Run("returns empty when no key matches", func(t *testing.T) {
		require.Empty(t, firstNonEmptyMetadataValue(md, "absent"))
	})
}

// x-user-id comes from Istio and must win over the user-id the BFF sets.
func TestFirstNonEmptyMetadataValueKeyPriority(t *testing.T) {
	md := metadata.Pairs(xUserIDKey, "from-istio", userIDKey, "from-bff")

	require.Equal(t, "from-istio", firstNonEmptyMetadataValue(md, xUserIDKey, userIDKey))
}

func TestClaimsFromMetadata(t *testing.T) {
	t.Run("no incoming metadata", func(t *testing.T) {
		require.Nil(t, claimsFromMetadata(context.Background()))
	})

	t.Run("no user id", func(t *testing.T) {
		require.Nil(t, claimsFromMetadata(incoming(xUserEmailKey, "a@example.com")))
	})

	t.Run("user id without email", func(t *testing.T) {
		claims := claimsFromMetadata(incoming(userIDKey, testUserID))

		require.NotNil(t, claims)
		require.Equal(t, testUserID, claims.Subject)
		require.Empty(t, claims.Email)
	})

	t.Run("user id with email", func(t *testing.T) {
		claims := claimsFromMetadata(incoming(xUserIDKey, testUserID, xUserEmailKey, "a@example.com"))

		require.NotNil(t, claims)
		require.Equal(t, testUserID, claims.Subject)
		require.Equal(t, "a@example.com", claims.Email)
	})
}

func TestResolveUserIdentity(t *testing.T) {
	t.Run("metadata is preferred over context", func(t *testing.T) {
		ctx := session.WithUserID(incoming(userIDKey, "from-metadata"), "from-context")

		userID, source, err := resolveUserIdentity(ctx, noopSpan())

		require.NoError(t, err)
		require.Equal(t, "from-metadata", userID)
		require.Equal(t, "metadata", source)
	})

	t.Run("falls back to context", func(t *testing.T) {
		ctx := session.WithUserID(context.Background(), testUserID)

		userID, source, err := resolveUserIdentity(ctx, noopSpan())

		require.NoError(t, err)
		require.Equal(t, testUserID, userID)
		require.Equal(t, "context", source)
	})

	t.Run("neither source yields an identity", func(t *testing.T) {
		_, source, err := resolveUserIdentity(context.Background(), noopSpan())

		require.ErrorIs(t, err, ErrServerMissingUserID)
		require.Equal(t, "context", source)
	})

	t.Run("blank metadata value falls through to context", func(t *testing.T) {
		ctx := session.WithUserID(incoming(userIDKey, "  "), testUserID)

		userID, source, err := resolveUserIdentity(ctx, noopSpan())

		require.NoError(t, err)
		require.Equal(t, testUserID, userID)
		require.Equal(t, "context", source)
	})
}

func TestSessionUnaryServerInterceptorSkipsHealthAndReflection(t *testing.T) {
	interceptor := SessionUnaryServerInterceptor()

	var called bool

	handler := func(ctx context.Context, _ any) (any, error) {
		called = true
		// A skipped method must not have an identity forced onto it.
		_, err := session.GetUserID(ctx)
		require.Error(t, err)

		return "ok", nil
	}

	// No identity anywhere: without the skip this would fail with Unauthenticated.
	resp, err := interceptor(
		context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/grpc.health.v1.Health/Check"},
		handler,
	)

	require.NoError(t, err)
	require.Equal(t, "ok", resp)
	require.True(t, called)
}

func TestSessionUnaryServerInterceptorPassesIdentityToHandler(t *testing.T) {
	interceptor := SessionUnaryServerInterceptor()

	handler := func(ctx context.Context, _ any) (any, error) {
		userID, err := session.GetUserID(ctx)
		require.NoError(t, err)
		require.Equal(t, testUserID, userID)

		claims, err := session.GetClaims(ctx)
		require.NoError(t, err)
		require.Equal(t, "a@example.com", claims.Email)

		return "ok", nil
	}

	resp, err := interceptor(
		incoming(xUserIDKey, testUserID, xUserEmailKey, "a@example.com"), nil,
		&grpc.UnaryServerInfo{FullMethod: testMethod},
		handler,
	)

	require.NoError(t, err)
	require.Equal(t, "ok", resp)
}

func TestSessionUnaryServerInterceptorRejectsAnonymousCall(t *testing.T) {
	interceptor := SessionUnaryServerInterceptor()

	handler := func(context.Context, any) (any, error) {
		t.Fatal("handler must not run without an identity")

		return nil, nil
	}

	_, err := interceptor(
		context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: testMethod},
		handler,
	)

	require.Error(t, err)
	require.Equal(t, grpcCodes.Unauthenticated, status.Code(err))
}

// The handler's own error must reach the caller unchanged.
func TestSessionUnaryServerInterceptorPropagatesHandlerError(t *testing.T) {
	interceptor := SessionUnaryServerInterceptor()
	sentinel := status.Error(grpcCodes.FailedPrecondition, "nope")

	_, err := interceptor(
		incoming(userIDKey, testUserID), nil,
		&grpc.UnaryServerInfo{FullMethod: testMethod},
		func(context.Context, any) (any, error) { return nil, sentinel },
	)

	require.Equal(t, grpcCodes.FailedPrecondition, status.Code(err))
}

// fakeServerStream is the minimum grpc.ServerStream the interceptor touches.
//
//nolint:containedctx // grpc.ServerStream carries its context; that is the interface.
type fakeServerStream struct {
	grpc.ServerStream

	ctx context.Context
}

func (s *fakeServerStream) Context() context.Context {
	return s.ctx
}

func TestSessionStreamServerInterceptorWrapsContext(t *testing.T) {
	interceptor := SessionStreamServerInterceptor()

	stream := &fakeServerStream{ctx: incoming(userIDKey, testUserID)}

	var seen string

	err := interceptor(nil, stream, &grpc.StreamServerInfo{FullMethod: testMethod},
		func(_ any, s grpc.ServerStream) error {
			userID, errGet := session.GetUserID(s.Context())
			require.NoError(t, errGet)

			seen = userID

			return nil
		})

	require.NoError(t, err)
	require.Equal(t, testUserID, seen)
	// The original stream must be left alone.
	_, errOriginal := session.GetUserID(stream.Context())
	require.Error(t, errOriginal)
}

func TestSessionStreamServerInterceptorSkipsHealth(t *testing.T) {
	interceptor := SessionStreamServerInterceptor()
	stream := &fakeServerStream{ctx: context.Background()}

	var called bool

	err := interceptor(nil, stream, &grpc.StreamServerInfo{FullMethod: "/grpc.health.v1.Health/Watch"},
		func(any, grpc.ServerStream) error {
			called = true

			return nil
		})

	require.NoError(t, err)
	require.True(t, called)
}

func TestSessionStreamServerInterceptorRejectsAnonymousCall(t *testing.T) {
	interceptor := SessionStreamServerInterceptor()
	stream := &fakeServerStream{ctx: context.Background()}

	err := interceptor(nil, stream, &grpc.StreamServerInfo{FullMethod: testMethod},
		func(any, grpc.ServerStream) error {
			t.Fatal("handler must not run without an identity")

			return nil
		})

	require.Error(t, err)
	require.Equal(t, grpcCodes.Unauthenticated, status.Code(err))
}

func TestWrappedServerStreamReturnsTheOverriddenContext(t *testing.T) {
	base := &fakeServerStream{ctx: context.Background()}
	wrappedCtx := session.WithUserID(context.Background(), testUserID)

	wrapped := &sessionWrappedServerStream{ServerStream: base, wrappedCtx: wrappedCtx}

	userID, err := session.GetUserID(wrapped.Context())
	require.NoError(t, err)
	require.Equal(t, testUserID, userID)
}
