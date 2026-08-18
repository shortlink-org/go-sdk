package mongo

import (
	"github.com/shortlink-org/go-sdk/db"
)

// driverName is the STORE_TYPE value this driver answers to.
const driverName = "mongo"

//nolint:gochecknoinits // driver registration
func init() {
	db.Register(driverName, func(deps db.Deps) (db.DB, error) {
		opts, err := db.DriverOptions[Option](deps)
		if err != nil {
			return nil, err
		}

		return New(deps.Cfg, opts...), nil
	})
}

// With addresses this driver's options to db.New, so that client code passes
// them without naming the driver or giving up type safety:
//
//	store, err := db.New(ctx, log, tracer, metrics, cfg,
//		mongo.With(mongo.WithClientOptions(fn)),
//	)
//
// The options apply only when STORE_TYPE selects this driver.
func With(opts ...Option) db.Option {
	boxed := make([]any, 0, len(opts))
	for _, opt := range opts {
		boxed = append(boxed, opt)
	}

	return db.DriverOption(driverName, boxed...)
}
