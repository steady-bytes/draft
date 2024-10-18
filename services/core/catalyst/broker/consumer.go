package broker

type (
	Consumer interface {
		Consume()
	}

	consumer struct{}
)

func NewConsumer() Consumer {
	return &consumer{}

}

func (c *consumer) Consume() {
}
