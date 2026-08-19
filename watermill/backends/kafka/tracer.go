package kafka

import (
	"github.com/IBM/sarama"
	"github.com/dnwe/otelsarama"
)

type SaramaTracer interface {
	WrapConsumer(consumer sarama.Consumer) sarama.Consumer
	WrapPartitionConsumer(consumer sarama.PartitionConsumer) sarama.PartitionConsumer
	WrapConsumerGroupHandler(handler sarama.ConsumerGroupHandler) sarama.ConsumerGroupHandler
	WrapSyncProducer(config *sarama.Config, producer sarama.SyncProducer) sarama.SyncProducer
}

type OTELSaramaTracer struct{}

func NewOTELSaramaTracer() OTELSaramaTracer {
	return OTELSaramaTracer{}
}

//nolint:ireturn // the interface is the library's own contract
func (t OTELSaramaTracer) WrapConsumer(c sarama.Consumer) sarama.Consumer {
	return otelsarama.WrapConsumer(c)
}

//nolint:ireturn // the interface is the library's own contract
func (t OTELSaramaTracer) WrapConsumerGroupHandler(h sarama.ConsumerGroupHandler) sarama.ConsumerGroupHandler {
	return otelsarama.WrapConsumerGroupHandler(h)
}

//nolint:ireturn // the interface is the library's own contract
func (t OTELSaramaTracer) WrapPartitionConsumer(pc sarama.PartitionConsumer) sarama.PartitionConsumer {
	return otelsarama.WrapPartitionConsumer(pc)
}

//nolint:ireturn // the interface is the library's own contract
func (t OTELSaramaTracer) WrapSyncProducer(cfg *sarama.Config, p sarama.SyncProducer) sarama.SyncProducer {
	return otelsarama.WrapSyncProducer(cfg, p)
}
