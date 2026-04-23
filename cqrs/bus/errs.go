package bus

import "errors"

var (
	errNilContext                 = errors.New("cqrs/bus: context must not be nil")
	errForwarderTopicRequiredTx   = errors.New("cqrs/bus: forwarder topic is required for WithTxAwareOutbox")
	errOutboxMissingDB            = errors.New("cqrs/bus: sql.DB or pgxpool.Pool must be provided")
	errOutboxMissingSubscriber    = errors.New("cqrs/bus: outbox subscriber is required")
	errOutboxMissingRealPublisher = errors.New("cqrs/bus: real publisher is required")
	errOutboxMissingLogger        = errors.New("cqrs/bus: logger is required")
	errOutboxMissingMeterProvider = errors.New("cqrs/bus: meter provider is required")
	errForwarderNotConfigured     = errors.New("cqrs/bus: outbox forwarder is not configured")
	errNilTxOutboxConfig          = errors.New("cqrs/bus: transactional outbox config is nil")
)
