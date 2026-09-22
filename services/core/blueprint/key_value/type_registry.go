package key_value

import (
	"fmt"
	"strings"
	"sync"

	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/anypb"
)

// typeDescriptorTypeURL is the type_url TypeDescriptor entries are themselves stored under.
// Derived from the compiled-in descriptor rather than hardcoded so it can never drift from the
// actual generated type.
var typeDescriptorTypeURL = "type.googleapis.com/" + string((&kvv1.TypeDescriptor{}).ProtoReflect().Descriptor().FullName())

// typeRegistry holds every registered proto type's descriptor in memory, keyed by type_url, so
// DecodeValues can dynamically decode arbitrary registered messages without Blueprint having been
// compiled against them.
//
// Populated two ways: register (called synchronously from RegisterType's RPC handler for
// fail-fast validation, and again from Apply for every Set of a TypeDescriptor -- including ones
// forwarded to the raft leader from a different node -- so every node in a multi-node Blueprint
// cluster ends up with the same cache, not just whichever one originally received the
// RegisterType call) and rehydrate (a one-time scan of already-persisted TypeDescriptor entries).
// rehydrate is necessary because this FSM's Restore is a no-op -- Badger, not raft's own log, is
// this store's durable source of truth, so a freshly started node has to read what's already
// there directly rather than relying on raft to replay history through Apply.
type typeRegistry struct {
	mu          sync.RWMutex
	descriptors map[string]protoreflect.MessageDescriptor

	rehydrateOnce sync.Once
}

func newTypeRegistry() *typeRegistry {
	return &typeRegistry{
		descriptors: make(map[string]protoreflect.MessageDescriptor),
	}
}

// register validates that file_descriptor_set actually contains type_url's message, then caches
// its resolved descriptor. Idempotent -- calling it again with the same descriptor just rebuilds
// and overwrites the same cache entry.
func (r *typeRegistry) register(descriptor *kvv1.TypeDescriptor) (protoreflect.MessageDescriptor, error) {
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(descriptor.GetFileDescriptorSet(), &set); err != nil {
		return nil, fmt.Errorf("failed to unmarshal file descriptor set: %w", err)
	}

	files, err := protodesc.NewFiles(&set)
	if err != nil {
		return nil, fmt.Errorf("failed to build file registry: %w", err)
	}

	name := strings.TrimPrefix(descriptor.GetTypeUrl(), "type.googleapis.com/")
	desc, err := files.FindDescriptorByName(protoreflect.FullName(name))
	if err != nil {
		return nil, fmt.Errorf("file descriptor set doesn't contain %q: %w", descriptor.GetTypeUrl(), err)
	}

	msgDesc, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, fmt.Errorf("%q is not a message type", descriptor.GetTypeUrl())
	}

	r.mu.Lock()
	r.descriptors[descriptor.GetTypeUrl()] = msgDesc
	r.mu.Unlock()

	return msgDesc, nil
}

func (r *typeRegistry) lookup(typeURL string) (protoreflect.MessageDescriptor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.descriptors[typeURL]
	return d, ok
}

// rehydrate does a one-time (per process) scan of every already-persisted TypeDescriptor entry
// and rebuilds the in-memory cache from it. Safe to call repeatedly -- only the first call does
// any work. Failures are swallowed rather than surfaced: a fresh node with a temporarily-empty
// cache degrades to "not decodable yet" (the existing safe fallback), not an error on every read.
func (r *typeRegistry) rehydrate(model Model) {
	r.rehydrateOnce.Do(func() {
		witness, err := anypb.New(&kvv1.TypeDescriptor{})
		if err != nil {
			return
		}
		entries, err := model.List(witness)
		if err != nil {
			return
		}
		for _, v := range entries {
			td := &kvv1.TypeDescriptor{}
			if err := v.UnmarshalTo(td); err != nil {
				continue
			}
			_, _ = r.register(td)
		}
	})
}

// decodeValues decodes raw message bytes for typeURL using the cached descriptor, keyed the same
// way the request was. A key is simply absent from the result whenever it couldn't be decoded --
// no descriptor registered, corrupt bytes, or a panic somewhere in the decode path -- never a
// whole-call error.
func (r *typeRegistry) decodeValues(typeURL string, values map[string][]byte) map[string]string {
	result := make(map[string]string, len(values))

	descriptor, ok := r.lookup(typeURL)
	if !ok {
		return result
	}

	for key, raw := range values {
		if json, ok := decodeOne(descriptor, raw); ok {
			result[key] = json
		}
	}
	return result
}

// decodeOne decodes a single message, recovering from any panic in dynamicpb's decode path (not
// expected in normal operation, but a hostile or corrupt payload is exactly the kind of input
// this must never trust to behave) so one bad entry degrades to "not decodable" instead of taking
// down the whole DecodeValues call -- or the process.
func decodeOne(descriptor protoreflect.MessageDescriptor, raw []byte) (json string, ok bool) {
	defer func() {
		if p := recover(); p != nil {
			json, ok = "", false
		}
	}()

	msg := dynamicpb.NewMessage(descriptor)
	if err := proto.Unmarshal(raw, msg); err != nil {
		return "", false
	}

	data, err := protojson.Marshal(msg)
	if err != nil {
		return "", false
	}

	return string(data), true
}
