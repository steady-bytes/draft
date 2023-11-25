package controller

import (
	"errors"

	"github.com/steady-bytes/draft/internal/host/model"
)

type (
	TestController interface {
		TestMe() error
	}

	testController struct {
		testModel model.TestModel
	}
)

func NewTestCtrl(testModel model.TestModel) TestController {
	return &testController{
		testModel: testModel,
	}
}

func (c *testController) TestMe() error {
	if err := c.testModel.SaveTest(); err != nil {
		return err
	}

	return errors.New("implement me")
}
