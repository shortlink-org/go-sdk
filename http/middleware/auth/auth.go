package auth_middleware

import (
	"context"
	"net/http"

	ory "github.com/ory/client-go"
	"github.com/shortlink-org/go-sdk/auth/session"
	"github.com/shortlink-org/go-sdk/config"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/shortlink-org/go-sdk/http/middleware/auth"

type contextKey struct{ name string }

var contextCookieKey = &contextKey{"cookie"}

type auth struct {
	ory    *ory.APIClient
	tracer trace.Tracer
	cfg    *config.Config
}

func Auth(cfg *config.Config) func(next http.Handler) http.Handler {
	cfg.SetDefault("AUTH_URI", "http://127.0.0.1:4433")

	oryConfig := ory.NewConfiguration()

	var serverConfig ory.ServerConfiguration

	serverConfig.URL = cfg.GetString("AUTH_URI")
	oryConfig.Servers = ory.ServerConfigurations{serverConfig}
	httpClient := new(http.Client)
	httpClient.Transport = otelhttp.NewTransport(http.DefaultTransport)
	oryConfig.HTTPClient = httpClient

	return auth{
		ory:    ory.NewAPIClient(oryConfig),
		tracer: otel.Tracer(tracerName),
		cfg:    cfg,
	}.middleware
}

func (a auth) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx, span := a.tracer.Start(request.Context(), "ory.kratos.session",
			trace.WithAttributes(attribute.String("component", "auth_middleware")),
			trace.WithSpanKind(trace.SpanKindClient),
		)
		defer span.End()

		cookies := request.Header.Get("Cookie")

		sess, resp, err := a.ory.FrontendAPI.ToSession(ctx).
			Cookie(cookies).
			Execute()
		if resp != nil {
			defer func() {
				closeErr := resp.Body.Close()
				if closeErr != nil {
					span.RecordError(closeErr)
				}
			}()
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
			http.Redirect(writer, request, a.cfg.GetString("AUTH_URI")+"/auth/login", http.StatusFound)
			return
		}

		// Enrich context
		ctx = context.WithValue(ctx, contextCookieKey, cookies)
		ctx = session.WithSession(ctx, sess)

		if identity, ok := sess.GetIdentityOk(); ok && identity != nil {
			if id := identity.GetId(); id != "" {
				ctx = session.WithUserID(ctx, id)
			}
		}

		// set the new context
		request = request.WithContext(ctx)

		next.ServeHTTP(writer, request)
	})
}

// GetCookie retrieves the cookie from the context.
func GetCookie(ctx context.Context) string {
	if v, ok := ctx.Value(contextCookieKey).(string); ok {
		return v
	}

	return ""
}
