package chassis

import (
	"context"
	"net/http"

	kvv1 "github.com/steady-bytes/draft/api/core/registry/key_value/v1"
	kvv1Connect "github.com/steady-bytes/draft/api/core/registry/key_value/v1/v1connect"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// WithRegisteredType tells Blueprint the schema for a proto message this process stores in its
// Key/Value store, so the Key/Value web UI can decode and render values of that type generically
// instead of showing an opaque byte count. Only ever needs the message itself -- msg is never
// sent, just reflected on to build its (and its dependencies') descriptor.
//
// Unlike WithRoute, this needs no WithRunner-after-Start() dance: it's an ordinary outbound call
// to Blueprint's KeyValueService, the same kind every other direct KV call in this codebase
// already makes synchronously from within the builder chain (via c.config.Entrypoint()). There's
// no "can't reach myself before I'm listening" problem here because the target (Blueprint) is a
// separate, already-running process for every caller -- registering one of Blueprint's own types
// with itself would hit that problem, but nothing in this codebase does that.
//
// Panics on failure, matching the fail-loud convention every other builder-chain registration
// call in this codebase already uses (Register, WithRoute) -- a service that explicitly asked for
// this and can't get it is misconfigured, not something to silently degrade past.
func (c *Runtime) WithRegisteredType(msg proto.Message) *Runtime {
	if err := c.registerType(msg); err != nil {
		c.logger.WithError(err).Panic("failed to register type")
	}
	return c
}

func (c *Runtime) registerType(msg proto.Message) error {
	var (
		ctx     = context.Background()
		md      = msg.ProtoReflect().Descriptor()
		typeURL = "type.googleapis.com/" + string(md.FullName())
		logger  = c.logger.WithField("type_url", typeURL)
	)

	set := collectFileDescriptorSet(md.ParentFile())
	data, err := proto.Marshal(set)
	if err != nil {
		logger.WithError(err).Error("failed to marshal file descriptor set")
		return err
	}

	_, err = kvv1Connect.NewKeyValueServiceClient(http.DefaultClient, c.config.Entrypoint(),
		connect.WithInterceptors(NewCallerServiceClientInterceptor())).
		RegisterType(ctx, connect.NewRequest(&kvv1.RegisterTypeRequest{
			Descriptor_: &kvv1.TypeDescriptor{
				TypeUrl:           typeURL,
				FileDescriptorSet: data,
			},
		}))
	if err != nil {
		logger.WithError(err).Error("failed to register type with blueprint")
		return err
	}

	logger.Info("successfully registered type")

	return nil
}

// collectFileDescriptorSet walks fd and every file it transitively imports (Go's linked-in proto
// registry already has the full dependency graph in memory for any message compiled into the
// binary, so this is a pure in-memory walk, no I/O) into one self-contained FileDescriptorSet --
// protodesc.NewFiles on the receiving end needs every transitively-referenced file present, not
// just the message's own. The post-order walk (a file's imports are appended before the file
// itself) means dependencies always precede their dependents in the result.
func collectFileDescriptorSet(fd protoreflect.FileDescriptor) *descriptorpb.FileDescriptorSet {
	var (
		seen  = make(map[string]bool)
		files []*descriptorpb.FileDescriptorProto
	)

	var walk func(protoreflect.FileDescriptor)
	walk = func(fd protoreflect.FileDescriptor) {
		if seen[fd.Path()] {
			return
		}
		seen[fd.Path()] = true

		imports := fd.Imports()
		for i := 0; i < imports.Len(); i++ {
			walk(imports.Get(i).FileDescriptor)
		}

		files = append(files, protodesc.ToFileDescriptorProto(fd))
	}
	walk(fd)

	return &descriptorpb.FileDescriptorSet{File: files}
}
