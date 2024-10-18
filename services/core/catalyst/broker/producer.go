package broker

type (
	Producer interface {
		Produce()
	}

	producer struct{}
)

func NewProducer() Producer {
	return &producer{}
}

func (p *producer) Produce() {
}
