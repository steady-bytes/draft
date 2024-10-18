package broker

type (
	Controller interface {
		Consumer
		Producer
	}

	controller struct {
		Producer
		Consumer
	}
)

func NewController() Controller {
	return &controller{
		NewProducer(), NewConsumer(),
	}
}
