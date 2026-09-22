package key_value

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	fsv1 "github.com/steady-bytes/draft/api/core/consensus/fsm/v1"
	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Cnt "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"
	"github.com/steady-bytes/draft/pkg/chassis"

	"connectrpc.com/connect"
	"github.com/hashicorp/raft"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type (
	Controller interface {
		chassis.ConsensusRegistrar
		raft.FSM

		KeyValue
	}

	KeyValue interface {
		// Delete and Set take ctx so their write-path spans (see startWriteSpan) nest
		// correctly under whatever RPC/heartbeat span is already in flight, and so
		// CallerServiceFromContext can recover who's pushing the update -- see rpc.go's
		// Set/Delete handlers (populate ctx from the caller-service header) and
		// service_discovery/controller.go's Synchronize (populates it from the process
		// registry's own Name).
		Delete(ctx context.Context, log chassis.Logger, key string, value T, timeout time.Duration) error
		Set(ctx context.Context, log chassis.Logger, key string, value T, timeout time.Duration) (*SetResponse, error)
		Get(log chassis.Logger, key string, value T) (T, error)
		List(log chassis.Logger, kind T) (map[string]T, error)
		ListKinds(log chassis.Logger) ([]KindSummary, error)
		// RegisterType validates and persists a proto type's descriptor so DecodeValues can
		// decode values of that type generically. Validation happens synchronously here (a bad
		// descriptor fails fast, before ever being persisted); the in-memory cache used for
		// decoding is actually kept current via Apply, so every node in a multi-node Blueprint
		// cluster ends up with it -- not just whichever one happened to receive this call.
		RegisterType(ctx context.Context, log chassis.Logger, descriptor *kvv1.TypeDescriptor) error
		// DecodeValues decodes raw message bytes for type_url using a previously registered
		// descriptor. Never errors -- a key with no registered descriptor, or that fails to
		// decode, is simply absent from the result.
		DecodeValues(log chassis.Logger, typeURL string, values map[string][]byte) map[string]string
		// TypeRegistered reports whether typeURL has a descriptor registered via RegisterType.
		// Used by the type mutation consumer (type_mutation_consumer.go) as an allowlist check --
		// only a type a process has actually opted into can be mutated over Catalyst.
		TypeRegistered(typeURL string) bool
	}

	SetResponse struct {
		Error error
		Data  interface{}
	}

	controller struct {
		model        Model
		raft         *raft.Raft
		typeRegistry *typeRegistry
	}
)

const (
	NullOperation = iota
	Set
	Delete
)

var (
	ErrFailedLSMLogBuild  = errors.New("failed to build the raft log from the key/value provided")
	ErrFailedAnyCast      = errors.New("failed to cast the value to anypb")
	ErrFailedToMarshal    = errors.New("failed to marshal payload")
	ErrFailedRegisterType = errors.New("failed to register type")
)

func NewController(model Model) Controller {
	return &controller{
		model:        model,
		typeRegistry: newTypeRegistry(),
	}
}

// Implement the the `draft.ConsensusRegister` interface so that the underlying infrastructure
// is put into place before the service is running. To run this service as a replicated service
// that can share, and agree on.
func (c *controller) RegisterConsensus(raftConn interface{}) error {
	if raftConn != nil {
		if raft, ok := raftConn.(*raft.Raft); ok {
			c.raft = raft
			return nil
		} else {
			return errors.New("failed to register raft with the service")
		}
	}
	return errors.New("raft connection is nill")
}

func (c *controller) LeadershipChange(log chassis.Logger, leader bool, address string) {
	if leader {
		log.Info("became leader")
		value, err := anypb.New(&kvv1.Value{
			Data: address,
		})
		if err != nil {
			log.WithError(err).Error("failed to create any type from value")
			return
		}
		// write the grpc address and port of the grpc service to raft. Self-initiated, not
		// on behalf of any external caller -- attributed to "blueprint" itself.
		ctx := WithCallerService(context.Background(), "blueprint")
		_, err = c.Set(ctx, log, "leader", value, 500*time.Millisecond)
		if err != nil {
			log.WithError(err).Error("failed to set leader address")
		}
	} else {
		log.Info("become follower")
	}
}

// Delete removes key from the store, replicated the same way Set is: forwarded to the raft
// leader if this node isn't it, then applied through raft.Apply so every node's local model sees
// the delete — not just whichever node happened to receive the RPC. (Previously this wrote
// straight to the local model, bypassing raft entirely; see
// docs/architecture/service-registry-identity.md for why that was unsafe for a multi-node
// cluster.)
func (c *controller) Delete(ctx context.Context, log chassis.Logger, key string, kind T, timeout time.Duration) error {
	callerService := CallerServiceFromContext(ctx)
	log = log.WithField("caller_service", callerService)

	if c.raft.State() != raft.Leader {
		log.Debug("forwarding delete request to leader")
		ctx, span := chassis.StartSpan(ctx, "blueprint.kv.forward_to_leader")
		span.SetAttribute("operation", "delete")
		span.SetAttribute("key", key)
		span.SetBusinessAttribute("caller_service", callerService)

		a, _ := anypb.New(&kvv1.Value{})
		anyValue, err := c.model.Get("leader", a)
		if err != nil {
			log.WithError(err).Error("failed to get leader address")
			span.End(err)
			return err
		}
		v := &kvv1.Value{}
		err = anypb.UnmarshalTo(anyValue, v, proto.UnmarshalOptions{})
		if err != nil {
			log.WithError(err).Error("failed to unmarshal leader value")
			span.End(err)
			return err
		}
		client := kvv1Cnt.NewKeyValueServiceClient(http.DefaultClient, v.Data,
			connect.WithInterceptors(chassis.NewTraceClientInterceptor()))

		req := connect.NewRequest(&kvv1.DeleteRequest{
			Key:   key,
			Value: kind,
		})
		// Forward the *original* caller's identity, not this (forwarding) node's own name --
		// the leader's Set/Delete handler should still attribute the write to whoever asked
		// for it, not to the follower that happened to relay it.
		req.Header().Set(chassis.CallerServiceHeaderName, callerService)
		_, err = client.Delete(ctx, req)
		if err != nil {
			log.WithError(err).Error("failed to forward delete request to leader")
			span.End(err)
			return err
		}

		span.End(nil)
		return nil
	}

	_, applySpan := chassis.StartSpan(ctx, "blueprint.kv.raft_apply")
	applySpan.SetAttribute("operation", "delete")
	applySpan.SetAttribute("key", key)
	applySpan.SetBusinessAttribute("caller_service", callerService)

	lsmLog, err := c.buildLSMLog(key, kind, fsv1.Operation_DELETE)
	if err != nil {
		log.Error(ErrFailedLSMLogBuild.Error())
		applySpan.End(err)
		return ErrFailedLSMLogBuild
	}

	future := c.raft.Apply(lsmLog, timeout)
	if err := future.Error(); err != nil {
		log.Error(err.Error())
		applySpan.End(err)
		return errors.New("failed to apply command")
	}

	res, ok := future.Response().(*SetResponse)
	if !ok {
		err := errors.New("failed to apply command")
		applySpan.End(err)
		return err
	}

	if res.Error != nil {
		log.Error(res.Error.Error())
		applySpan.End(res.Error)
		return res.Error
	}

	applySpan.End(nil)
	return nil
}

func (c *controller) Get(log chassis.Logger, key string, value T) (T, error) {
	val, err := c.model.Get(key, value)
	if err != nil {
		log.Error(err.Error())
		return nil, err
	}
	return val, nil
}

func (c *controller) Set(ctx context.Context, log chassis.Logger, key string, value T, timeout time.Duration) (*SetResponse, error) {
	callerService := CallerServiceFromContext(ctx)
	log = log.WithField("caller_service", callerService)

	// forward the set request to the leader if we are not the leader
	if c.raft.State() != raft.Leader {
		log.Debug("forwarding set request to leader")
		ctx, span := chassis.StartSpan(ctx, "blueprint.kv.forward_to_leader")
		span.SetAttribute("operation", "set")
		span.SetAttribute("key", key)
		span.SetBusinessAttribute("caller_service", callerService)

		// create a client to the current leader
		a, _ := anypb.New(&kvv1.Value{})
		anyValue, err := c.model.Get("leader", a)
		if err != nil {
			log.WithError(err).Error("failed to get leader address")
			span.End(err)
			return nil, err
		}
		v := &kvv1.Value{}
		err = anypb.UnmarshalTo(anyValue, v, proto.UnmarshalOptions{})
		if err != nil {
			log.WithError(err).Error("failed to unmarshal leader value")
			span.End(err)
			return nil, err
		}
		client := kvv1Cnt.NewKeyValueServiceClient(http.DefaultClient, v.Data,
			connect.WithInterceptors(chassis.NewTraceClientInterceptor()))

		// forward the set request to the leader
		req := connect.NewRequest(&kvv1.SetRequest{
			Key:   key,
			Value: value,
		})
		// Forward the *original* caller's identity, not this (forwarding) node's own name --
		// the leader's Set/Delete handler should still attribute the write to whoever asked
		// for it, not to the follower that happened to relay it.
		req.Header().Set(chassis.CallerServiceHeaderName, callerService)
		_, err = client.Set(ctx, req)
		if err != nil {
			log.WithError(err).Error("failed to forward set request to leader")
			span.End(err)
			return nil, err
		}

		span.End(nil)
		return nil, nil
	}

	_, applySpan := chassis.StartSpan(ctx, "blueprint.kv.raft_apply")
	applySpan.SetAttribute("operation", "set")
	applySpan.SetAttribute("key", key)
	applySpan.SetBusinessAttribute("caller_service", callerService)

	// build lsm log
	lsmLog, err := c.buildLSMLog(key, value, fsv1.Operation_SET)
	if err != nil {
		log.Error(ErrFailedLSMLogBuild.Error())
		applySpan.End(err)
		return nil, ErrFailedLSMLogBuild
	}

	future := c.raft.Apply(lsmLog, timeout)
	if err := future.Error(); err != nil {
		log.Error(err.Error())
		applySpan.End(err)
		return nil, errors.New("failed to apply command")
	}

	res, ok := future.Response().(*SetResponse)
	if !ok {
		err := errors.New("failed to apply command")
		applySpan.End(err)
		return nil, err
	}

	if res.Error != nil {
		log.Error(res.Error.Error())
		applySpan.End(res.Error)
		return nil, res.Error
	}

	applySpan.End(nil)
	return res, nil
}

func (c *controller) buildLSMLog(key string, value T, operation fsv1.Operation) ([]byte, error) {
	payload := &fsv1.CommandPayload{
		Operation: operation,
		Key:       key,
		Value:     value,
	}

	data, err := proto.Marshal(payload)
	if err != nil {
		return nil, ErrFailedToMarshal
	}

	return data, nil
}

func (c *controller) List(log chassis.Logger, kind T) (map[string]T, error) {
	keyValMap, err := c.model.List(kind)
	if err != nil {
		log.Error(err.Error())
		return nil, ErrFailedList
	}

	return keyValMap, nil
}

func (c *controller) ListKinds(log chassis.Logger) ([]KindSummary, error) {
	kinds, err := c.model.ListKinds()
	if err != nil {
		log.Error(err.Error())
		return nil, ErrFailedListKinds
	}

	return kinds, nil
}

func (c *controller) RegisterType(ctx context.Context, log chassis.Logger, descriptor *kvv1.TypeDescriptor) error {
	if _, err := c.typeRegistry.register(descriptor); err != nil {
		log.WithError(err).Error(ErrFailedRegisterType.Error())
		return err
	}

	value, err := anypb.New(descriptor)
	if err != nil {
		log.WithError(err).Error(ErrFailedAnyCast.Error())
		return ErrFailedAnyCast
	}

	// Goes through Set (not model.Set directly) so this forwards to the raft leader like any
	// other write when this node isn't it -- registering a type is a replicated KV write like
	// everything else in this store, not a local-only operation.
	if _, err := c.Set(ctx, log, descriptor.GetTypeUrl(), value, 500*time.Millisecond); err != nil {
		log.WithError(err).Error(ErrFailedRegisterType.Error())
		return err
	}

	return nil
}

func (c *controller) TypeRegistered(typeURL string) bool {
	// Same lazy-rehydrate-then-lookup sequence as DecodeValues -- this node's in-memory cache may
	// still be empty if nothing has called DecodeValues (or TypeRegistered) on it yet, since FSM
	// Restore is a no-op (see rehydrate's own doc comment).
	c.typeRegistry.rehydrate(c.model)
	_, ok := c.typeRegistry.lookup(typeURL)
	return ok
}

func (c *controller) DecodeValues(log chassis.Logger, typeURL string, values map[string][]byte) map[string]string {
	// Lazy, once-per-process: this store's FSM Restore is a no-op (Badger, not raft's log, is the
	// durable source of truth here), so a freshly started node needs to read what's already
	// persisted directly rather than relying on raft to replay history through Apply.
	c.typeRegistry.rehydrate(c.model)
	return c.typeRegistry.decodeValues(typeURL, values)
}

///////////////
// == FSM == //
///////////////

// Implement the `FSM` interface for the `key/value` store so that a change to the leader node will
// be written to each other node joined to the cluster.

// Apply is called when the leader of the cluster received a command that needs to be sent to all of
// the followers in the cluster. Currently in our case the `Set`, and `Delete` invocations of the
// `key/value` store will consume `Apply`
func (c *controller) Apply(log *raft.Log) interface{} {
	switch log.Type {
	case raft.LogCommand:
		var (
			payload = fsv1.CommandPayload{}
			err     error
		)
		if err = proto.Unmarshal(log.Data, &payload); err != nil {
			return &SetResponse{
				Error: errors.New("error marshalling value payload"),
				Data:  payload,
			}
		}

		switch payload.Operation {
		case Delete:
			if err := c.model.Delete(payload.Key, payload.Value); err != nil {
				return &SetResponse{
					Error: errors.New("failed to delete key/val"),
					Data:  payload,
				}
			}
			return &SetResponse{
				Error: nil,
				Data:  payload,
			}
		case Set:
			if err := c.model.Set(payload.Key, payload.Value); err != nil {
				return &SetResponse{
					Error: errors.New("failed to set key/val"),
					Data:  payload,
				}
			} else {
				// Every node applies every committed Set (that's the whole point of raft) --
				// this is the one code path guaranteed to run on all of them, including
				// followers that never personally handled the originating RegisterType RPC, so
				// it's where the in-memory type registry cache actually gets kept current
				// cluster-wide. register is idempotent, so this is harmless even on the node
				// that already cached it synchronously in RegisterType.
				if payload.GetValue().GetTypeUrl() == typeDescriptorTypeURL {
					td := &kvv1.TypeDescriptor{}
					if err := payload.GetValue().UnmarshalTo(td); err == nil {
						_, _ = c.typeRegistry.register(td)
					}
				}
				return &SetResponse{
					Error: nil,
					Data:  payload,
				}
			}
		case NullOperation:
			fmt.Println("null operation received from log")
			return nil
		}
	}

	return nil
}

// TODO -> figure out how to implement this
func (c *controller) Snapshot() (raft.FSMSnapshot, error) {
	return newSnapshotNoop()
}

// TODO -> figure out how to implement this
func (c *controller) Restore(rClose io.ReadCloser) error {
	return nil
}

// snapshotNoop handle noop snapshot
type snapshotNoop struct{}

// Persist persist to disk. Return nil on success, otherwise return error.
func (s snapshotNoop) Persist(_ raft.SnapshotSink) error { return nil }

// Release release the lock after persist snapshot.
// Release is invoked when we are finished with the snapshot.
func (s snapshotNoop) Release() {}

// newSnapshotNoop is returned by an FSM in response to a snapshotNoop
// It must be safe to invoke FSMSnapshot methods with concurrent
// calls to Apply.
func newSnapshotNoop() (raft.FSMSnapshot, error) {
	return &snapshotNoop{}, nil
}
