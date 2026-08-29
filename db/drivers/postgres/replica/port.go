package replica

import (
	"context"
	"time"

	"github.com/shortlink-org/go-sdk/db/drivers/postgres/replica/wal"
)

// TextPort adapts the router to string-shaped interfaces.
//
// It exists so that other modules can consume the read-your-writes guarantee
// without importing this one. The watermill module declares two-method
// interfaces of its own and takes whatever satisfies them; Go's structural
// typing does the rest. That keeps the heavy dependency graph of db — mongo,
// badger, clickhouse, scylla — out of a module that only needs to pass a
// string from a publisher to a subscriber.
//
// The string is a token, so it survives a failover: see Router.ResolveToken.
type TextPort struct {
	router *Router
}

// Port returns the router's string-shaped adapter.
//
//	client.Publisher = watermill.NewConsistencyPublisher(client.Publisher, router.Port(), log, meter)
//	client.Router.AddMiddleware(watermill.NewConsistencyMiddleware(router.Port(), opts, meter))
func (r *Router) Port() *TextPort {
	return &TextPort{router: r}
}

// Capture returns a token describing the writes made on ctx. An empty token
// means nothing was written; an error means a write was observed but its
// position could not be captured.
//
// A context that has not written returns "" and costs nothing: the round
// trip to the primary happens only when there is a guarantee to hand over.
func (p *TextPort) Capture(ctx context.Context) (string, error) {
	tracker := TrackerFromContext(ctx)
	if !tracker.Tainted() && tracker.Watermark() == 0 {
		return "", nil
	}

	token, err := p.router.Token(ctx)
	if err != nil {
		return "", err
	}

	return token.String(), nil
}

// Await reports whether a replica can serve reads that must observe the token,
// waiting up to maxWait for it to become so.
//
// A token this process cannot interpret — one from another timeline, after a
// failover — is not an error and not a reason to wait. It is a reason to read
// from the primary, which Apply arranges.
func (p *TextPort) Await(ctx context.Context, token string, maxWait time.Duration) (bool, error) {
	parsed, err := wal.ParseToken(token)
	if err != nil {
		return true, nil //nolint:nilerr // an unreadable token is handled by Apply, not by waiting
	}

	resolution := p.router.ResolveToken(parsed)
	if resolution.State() != TokenAccepted {
		return true, nil
	}

	if !p.router.Enabled() {
		return true, nil
	}

	return p.router.gate.await(ctx, resolution.Position(), maxWait)
}

// Apply returns a context carrying the guarantee the token describes, so that
// reads made downstream are routed accordingly.
func (p *TextPort) Apply(ctx context.Context, token string) context.Context {
	parsed, err := wal.ParseToken(token)
	if err != nil {
		// A watermark we were sent but cannot read still says the producer
		// wrote something. Reading from a replica would be a guess.
		return OnPrimary(ctx)
	}

	return p.router.WithToken(ctx, parsed)
}

// Scope returns a context whose reads may use a replica.
//
// A unit of work that arrives carrying no watermark still has to be scoped, or
// the no-tracker policy sends every one of its reads to the primary and the
// boundary buys nothing.
func (p *TextPort) Scope(ctx context.Context) context.Context {
	return WithTracker(ctx)
}

// Observe raises the watermark of the tracker already in ctx.
//
// It is the return half of a synchronous hop: a callee that wrote on our behalf
// hands its position back, and our own later reads must not run behind it.
// Apply cannot serve this — it builds a new context, and by the time a reply
// comes back the caller is already holding theirs.
//
// A token that cannot be read taints instead. The callee said it wrote; not
// being able to say how far is a reason to use the primary, not a reason to
// forget.
func (p *TextPort) Observe(ctx context.Context, token string) {
	tracker := TrackerFromContext(ctx)
	if tracker == nil {
		return
	}

	parsed, err := wal.ParseToken(token)
	if err != nil {
		tracker.Taint()

		return
	}

	resolution := p.router.ResolveToken(parsed)
	if resolution.State() != TokenAccepted {
		tracker.Taint()

		return
	}

	tracker.Observe(resolution.Position())
}
