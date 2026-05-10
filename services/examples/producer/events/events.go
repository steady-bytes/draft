package events

import (
	"time"

	"github.com/google/uuid"
	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	crudv1 "github.com/steady-bytes/draft/api/examples/crud/v1"
	userv1 "github.com/steady-bytes/draft/api/examples/user/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const source = "/examples/producer"

func timeAttr() *acv1.CloudEvent_CloudEventAttributeValue {
	return &acv1.CloudEvent_CloudEventAttributeValue{
		Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeTimestamp{
			CeTimestamp: timestamppb.Now(),
		},
	}
}

func subjectAttr(s string) *acv1.CloudEvent_CloudEventAttributeValue {
	return &acv1.CloudEvent_CloudEventAttributeValue{
		Attr: &acv1.CloudEvent_CloudEventAttributeValue_CeString{
			CeString: s,
		},
	}
}

func NewDatabaseModelSaved(modelID, modelName string, op crudv1.Operation) (*acv1.CloudEvent, error) {
	b, err := protojson.Marshal(&crudv1.DatabaseModelSaved{
		ModelId:   modelID,
		ModelName: modelName,
		Operation: op,
	})
	if err != nil {
		return nil, err
	}
	return &acv1.CloudEvent{
		Id:          uuid.NewString(),
		Source:      source,
		SpecVersion: "1.0",
		Type:        "examples.crud.v1.DatabaseModelSaved",
		Data:        &acv1.CloudEvent_TextData{TextData: string(b)},
		Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{
			"time":    timeAttr(),
			"subject": subjectAttr(modelID),
		},
	}, nil
}

func NewUserCreated(userID, email, name string) (*acv1.CloudEvent, error) {
	b, err := protojson.Marshal(&userv1.UserCreated{
		UserId: userID,
		Email:  email,
		Name:   name,
	})
	if err != nil {
		return nil, err
	}
	return &acv1.CloudEvent{
		Id:          uuid.NewString(),
		Source:      source,
		SpecVersion: "1.0",
		Type:        "examples.user.v1.UserCreated",
		Data:        &acv1.CloudEvent_TextData{TextData: string(b)},
		Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{
			"time":    timeAttr(),
			"subject": subjectAttr(userID),
		},
	}, nil
}

func NewUserLoggedIn(userID, email string) (*acv1.CloudEvent, error) {
	b, err := protojson.Marshal(&userv1.UserLoggedIn{
		UserId:  userID,
		Email:   email,
		LoginAt: timestamppb.New(time.Now()),
	})
	if err != nil {
		return nil, err
	}
	return &acv1.CloudEvent{
		Id:          uuid.NewString(),
		Source:      source,
		SpecVersion: "1.0",
		Type:        "examples.user.v1.UserLoggedIn",
		Data:        &acv1.CloudEvent_TextData{TextData: string(b)},
		Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{
			"time":    timeAttr(),
			"subject": subjectAttr(userID),
		},
	}, nil
}
