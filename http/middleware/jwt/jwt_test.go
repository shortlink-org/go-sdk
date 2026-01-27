package jwt_middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/shortlink-org/go-sdk/auth/session"
	"github.com/shortlink-org/go-sdk/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestToken(claims oathkeeperClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	// Sign with a dummy key (we don't verify signatures)
	tokenString, _ := token.SignedString([]byte("test-secret"))
	return tokenString
}

func TestJWT_ValidToken(t *testing.T) {
	cfg := config.New()

	claims := oathkeeperClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-123",
			Issuer:    "https://shortlink.best",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Email:      "test@example.com",
		Name:       "Test User",
		IdentityID: "identity-456",
		SessionID:  "session-789",
	}

	tokenString := createTestToken(claims)

	handler := JWT(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify claims are in context
		sessionClaims, err := session.GetClaims(r.Context())
		require.NoError(t, err)
		assert.Equal(t, "user-123", sessionClaims.Subject)
		assert.Equal(t, "test@example.com", sessionClaims.Email)
		assert.Equal(t, "Test User", sessionClaims.Name)

		// Verify user ID is in context
		userID, err := session.GetUserID(r.Context())
		require.NoError(t, err)
		assert.Equal(t, "user-123", userID)

		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestJWT_MissingToken(t *testing.T) {
	cfg := config.New()

	handler := JWT(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Accept", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWT_InvalidToken(t *testing.T) {
	cfg := config.New()

	handler := JWT(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer invalid.token.here")
	req.Header.Set("Accept", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWT_MissingSubject(t *testing.T) {
	cfg := config.New()

	claims := oathkeeperClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "", // Empty subject
			Issuer:  "https://shortlink.best",
		},
		Email: "test@example.com",
	}

	tokenString := createTestToken(claims)

	handler := JWT(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	req.Header.Set("Accept", "application/json")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestJWT_RedirectForBrowser(t *testing.T) {
	cfg := config.New()
	cfg.SetDefault("AUTH_LOGIN_URL", "/auth/login")

	handler := JWT(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Accept", "text/html")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, "/auth/login", rec.Header().Get("Location"))
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected string
	}{
		{"valid bearer", "Bearer abc123", "abc123"},
		{"bearer lowercase", "bearer abc123", "abc123"},
		{"BEARER uppercase", "BEARER abc123", "abc123"},
		{"empty", "", ""},
		{"no bearer prefix", "abc123", ""},
		{"basic auth", "Basic abc123", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			result := extractBearerToken(req)
			assert.Equal(t, tt.expected, result)
		})
	}
}
