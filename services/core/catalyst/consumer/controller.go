package consumer

type (
	Controller interface {
		Producer
	}

	Producer interface {
		Produce()
	}

	controller struct{}
)

func NewController() Controller {
	return &controller{}
}

func (c *controller) Produce() {
}
