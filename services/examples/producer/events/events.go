package events

import (
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
	acv1 "github.com/steady-bytes/draft/api/core/message_broker/actors/v1"
	crudv1 "github.com/steady-bytes/draft/api/examples/crud/v1"
	userv1 "github.com/steady-bytes/draft/api/examples/user/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ─── Pools ────────────────────────────────────────────────────────────────────

type modelKind struct {
	name   string
	source string
}

var modelPool = []modelKind{
	{"User", "/services/user-service"},
	{"Product", "/services/catalog-service"},
	{"Order", "/services/order-service"},
	{"Cart", "/services/cart-service"},
	{"Invoice", "/services/billing-service"},
	{"Payment", "/services/payment-service"},
	{"Shipment", "/services/fulfillment-service"},
	{"Review", "/services/review-service"},
}

type fakeUser struct {
	name  string
	email string
}

var userPool = []fakeUser{
	{"Alice Martin", "alice.martin@example.com"},
	{"Bob Chen", "bob.chen@example.com"},
	{"Carol Davis", "carol.davis@example.com"},
	{"Daniel Kim", "daniel.kim@example.com"},
	{"Emma Wilson", "emma.wilson@example.com"},
	{"Frank Torres", "frank.torres@example.com"},
	{"Grace Lee", "grace.lee@example.com"},
	{"Henry Brown", "henry.brown@example.com"},
	{"Isabel Garcia", "isabel.garcia@example.com"},
	{"Jack Murphy", "jack.murphy@example.com"},
}

// operations weighted toward CREATE and UPDATE since deletes are rarer.
var weightedOps = []crudv1.Operation{
	crudv1.Operation_OPERATION_CREATE,
	crudv1.Operation_OPERATION_CREATE,
	crudv1.Operation_OPERATION_CREATE,
	crudv1.Operation_OPERATION_UPDATE,
	crudv1.Operation_OPERATION_UPDATE,
	crudv1.Operation_OPERATION_DELETE,
}

// ─── Random generators ────────────────────────────────────────────────────────

// Random returns a randomly-selected CloudEvent drawn from the available event
// types. The distribution is weighted: 50 % DatabaseModelSaved, 30 %
// UserLoggedIn, 20 % UserCreated.
func Random() (*acv1.CloudEvent, error) {
	n := rand.IntN(10)
	switch {
	case n < 5:
		return RandomDatabaseModelSaved()
	case n < 8:
		return RandomUserLoggedIn()
	default:
		return RandomUserCreated()
	}
}

func RandomDatabaseModelSaved() (*acv1.CloudEvent, error) {
	m := modelPool[rand.IntN(len(modelPool))]
	op := weightedOps[rand.IntN(len(weightedOps))]
	return NewDatabaseModelSaved(uuid.NewString(), m.name, op, m.source)
}

func RandomUserCreated() (*acv1.CloudEvent, error) {
	u := userPool[rand.IntN(len(userPool))]
	return NewUserCreated(uuid.NewString(), u.email, u.name)
}

func RandomUserLoggedIn() (*acv1.CloudEvent, error) {
	u := userPool[rand.IntN(len(userPool))]
	return NewUserLoggedIn(uuid.NewString(), u.email)
}

// ─── Explicit constructors ────────────────────────────────────────────────────

func NewDatabaseModelSaved(modelID, modelName string, op crudv1.Operation, source string) (*acv1.CloudEvent, error) {
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
		Source:      "/services/user-service",
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
		Source:      "/services/user-service",
		SpecVersion: "1.0",
		Type:        "examples.user.v1.UserLoggedIn",
		Data:        &acv1.CloudEvent_TextData{TextData: string(b)},
		Attributes: map[string]*acv1.CloudEvent_CloudEventAttributeValue{
			"time":    timeAttr(),
			"subject": subjectAttr(userID),
		},
	}, nil
}

// ─── Attribute helpers ────────────────────────────────────────────────────────

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
