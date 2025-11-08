package auth_middleware

import (
	"context"
	"net/http"

	ory "github.com/ory/client-go"
	"github.com/spf13/viper"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/shortlink-org/go-sdk/auth/session"
)

const tracerName = "github.com/shortlink-org/go-sdk/http/middleware/auth"

type contextKey struct{ name string }

var contextCookieKey = &contextKey{"cookie"}

type auth struct {
	ory    *ory.APIClient
	tracer trace.Tracer
}

func Auth() func(next http.Handler) http.Handler {
	viper.SetDefault("AUTH_URI", "http://127.0.0.1:4433")

	c := ory.NewConfiguration()
	c.Servers = ory.ServerConfigurations{{URL: viper.GetString("AUTH_URI")}}
	c.HTTPClient = &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	return auth{
		ory:    ory.NewAPIClient(c),
		tracer: otel.Tracer(tracerName),
	}.middleware
}

func (a auth) middleware(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ctx, span := a.tracer.Start(r.Context(), "ory.kratos.session",
			trace.WithAttributes(attribute.String("component", "auth_middleware")),
		)
		defer span.End()

		cookies := r.Header.Get("Cookie")

		sess, resp, err := a.ory.FrontendAPI.ToSession(ctx).Cookie(cookies).Execute()
		if resp != nil {
			defer resp.Body.Close()
		}

		switch {
		case err != nil:
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		case sess == nil:
			span.SetStatus(codes.Error, "session payload is empty")
		case sess.Active == nil:
			span.SetStatus(codes.Error, "session active flag missing")
		case !*sess.Active:
			span.SetStatus(codes.Error, "session inactive")
		default:
			span.SetStatus(codes.Ok, "session validated")
		}

		if err != nil || sess == nil || sess.Active == nil || !*sess.Active {
			// this will redirect the user to the login page if the cookie is invalid
			// NOTE:
			// 	- we use 302 instead of 303 because proxy servers might not understand the 303 status code
			// details -> https://stackoverflow.com/questions/2839585/what-is-correct-http-status-code-when-redirecting-to-a-login-page
			http.Redirect(w, r, viper.GetString("AUTH_URI")+"/auth/login", http.StatusFound)
			return
		}

		ctx = context.WithValue(ctx, contextCookieKey, cookies)
		ctx = session.WithSession(ctx, sess)

		if identity, ok := sess.GetIdentityOk(); ok && identity != nil {
			if id := identity.GetId(); id != "" {
				ctx = session.WithUserID(ctx, id)
			}
		}

		// set the new context
		r = r.WithContext(ctx)

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

// GetCookie retrieves the cookie from the context.
func GetCookie(ctx context.Context) string {
	if v, ok := ctx.Value(contextCookieKey).(string); ok {
		return v
	}

	return ""
}
