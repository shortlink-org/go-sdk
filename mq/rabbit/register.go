package rabbit

import (
	"github.com/shortlink-org/go-sdk/mq"
)

// driverName is the MQ_TYPE value this driver answers to.
const driverName = "rabbitmq"

//nolint:gochecknoinits // driver registration
func init() {
	mq.Register(driverName, func(deps mq.Deps) (mq.MQ, error) {
		return New(deps.Log, deps.Cfg), nil
	})
}
