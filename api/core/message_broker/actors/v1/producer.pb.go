// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.34.2
// 	protoc        (unknown)
// source: core/message_broker/actors/v1/producer.proto

package v1

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// Send this `Message` to the other `Actors` in the system that are subscribed to this `Message`
type ProduceRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Message *CloudEvent `protobuf:"bytes,1,opt,name=message,proto3" json:"message" yaml:"message" csv:"message" pg:"message" bun:"message"`
}

func (x *ProduceRequest) Reset() {
	*x = ProduceRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_core_message_broker_actors_v1_producer_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ProduceRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ProduceRequest) ProtoMessage() {}

func (x *ProduceRequest) ProtoReflect() protoreflect.Message {
	mi := &file_core_message_broker_actors_v1_producer_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ProduceRequest.ProtoReflect.Descriptor instead.
func (*ProduceRequest) Descriptor() ([]byte, []int) {
	return file_core_message_broker_actors_v1_producer_proto_rawDescGZIP(), []int{0}
}

func (x *ProduceRequest) GetMessage() *CloudEvent {
	if x != nil {
		return x.Message
	}
	return nil
}

type ProduceResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// The message id is returned as a way to acknowledge the message as been committed
	Id string `protobuf:"bytes,1,opt,name=id,proto3" json:"id" pg:"id" bun:"id" yaml:"id" csv:"id"`
}

func (x *ProduceResponse) Reset() {
	*x = ProduceResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_core_message_broker_actors_v1_producer_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ProduceResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ProduceResponse) ProtoMessage() {}

func (x *ProduceResponse) ProtoReflect() protoreflect.Message {
	mi := &file_core_message_broker_actors_v1_producer_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ProduceResponse.ProtoReflect.Descriptor instead.
func (*ProduceResponse) Descriptor() ([]byte, []int) {
	return file_core_message_broker_actors_v1_producer_proto_rawDescGZIP(), []int{1}
}

func (x *ProduceResponse) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

var File_core_message_broker_actors_v1_producer_proto protoreflect.FileDescriptor

var file_core_message_broker_actors_v1_producer_proto_rawDesc = []byte{
	0x0a, 0x2c, 0x63, 0x6f, 0x72, 0x65, 0x2f, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x5f, 0x62,
	0x72, 0x6f, 0x6b, 0x65, 0x72, 0x2f, 0x61, 0x63, 0x74, 0x6f, 0x72, 0x73, 0x2f, 0x76, 0x31, 0x2f,
	0x70, 0x72, 0x6f, 0x64, 0x75, 0x63, 0x65, 0x72, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x1d,
	0x63, 0x6f, 0x72, 0x65, 0x2e, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x5f, 0x62, 0x72, 0x6f,
	0x6b, 0x65, 0x72, 0x2e, 0x61, 0x63, 0x74, 0x6f, 0x72, 0x73, 0x2e, 0x76, 0x31, 0x1a, 0x2a, 0x63,
	0x6f, 0x72, 0x65, 0x2f, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x5f, 0x62, 0x72, 0x6f, 0x6b,
	0x65, 0x72, 0x2f, 0x61, 0x63, 0x74, 0x6f, 0x72, 0x73, 0x2f, 0x76, 0x31, 0x2f, 0x6d, 0x6f, 0x64,
	0x65, 0x6c, 0x73, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x22, 0x55, 0x0a, 0x0e, 0x50, 0x72, 0x6f,
	0x64, 0x75, 0x63, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x43, 0x0a, 0x07, 0x6d,
	0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x29, 0x2e, 0x63,
	0x6f, 0x72, 0x65, 0x2e, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x5f, 0x62, 0x72, 0x6f, 0x6b,
	0x65, 0x72, 0x2e, 0x61, 0x63, 0x74, 0x6f, 0x72, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x43, 0x6c, 0x6f,
	0x75, 0x64, 0x45, 0x76, 0x65, 0x6e, 0x74, 0x52, 0x07, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65,
	0x22, 0x21, 0x0a, 0x0f, 0x50, 0x72, 0x6f, 0x64, 0x75, 0x63, 0x65, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x02, 0x69, 0x64, 0x32, 0x7a, 0x0a, 0x08, 0x50, 0x72, 0x6f, 0x64, 0x75, 0x63, 0x65, 0x72, 0x12,
	0x6e, 0x0a, 0x07, 0x50, 0x72, 0x6f, 0x64, 0x75, 0x63, 0x65, 0x12, 0x2d, 0x2e, 0x63, 0x6f, 0x72,
	0x65, 0x2e, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x5f, 0x62, 0x72, 0x6f, 0x6b, 0x65, 0x72,
	0x2e, 0x61, 0x63, 0x74, 0x6f, 0x72, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x50, 0x72, 0x6f, 0x64, 0x75,
	0x63, 0x65, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x2e, 0x2e, 0x63, 0x6f, 0x72, 0x65,
	0x2e, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67, 0x65, 0x5f, 0x62, 0x72, 0x6f, 0x6b, 0x65, 0x72, 0x2e,
	0x61, 0x63, 0x74, 0x6f, 0x72, 0x73, 0x2e, 0x76, 0x31, 0x2e, 0x50, 0x72, 0x6f, 0x64, 0x75, 0x63,
	0x65, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x28, 0x01, 0x30, 0x01, 0x42,
	0x41, 0x5a, 0x3f, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x73, 0x74,
	0x65, 0x61, 0x64, 0x79, 0x2d, 0x62, 0x79, 0x74, 0x65, 0x73, 0x2f, 0x64, 0x72, 0x61, 0x66, 0x74,
	0x2f, 0x61, 0x70, 0x69, 0x2f, 0x63, 0x6f, 0x72, 0x65, 0x2f, 0x6d, 0x65, 0x73, 0x73, 0x61, 0x67,
	0x65, 0x5f, 0x62, 0x72, 0x6f, 0x6b, 0x65, 0x72, 0x2f, 0x61, 0x63, 0x74, 0x6f, 0x72, 0x73, 0x2f,
	0x76, 0x31, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_core_message_broker_actors_v1_producer_proto_rawDescOnce sync.Once
	file_core_message_broker_actors_v1_producer_proto_rawDescData = file_core_message_broker_actors_v1_producer_proto_rawDesc
)

func file_core_message_broker_actors_v1_producer_proto_rawDescGZIP() []byte {
	file_core_message_broker_actors_v1_producer_proto_rawDescOnce.Do(func() {
		file_core_message_broker_actors_v1_producer_proto_rawDescData = protoimpl.X.CompressGZIP(file_core_message_broker_actors_v1_producer_proto_rawDescData)
	})
	return file_core_message_broker_actors_v1_producer_proto_rawDescData
}

var file_core_message_broker_actors_v1_producer_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
var file_core_message_broker_actors_v1_producer_proto_goTypes = []any{
	(*ProduceRequest)(nil),  // 0: core.message_broker.actors.v1.ProduceRequest
	(*ProduceResponse)(nil), // 1: core.message_broker.actors.v1.ProduceResponse
	(*CloudEvent)(nil),      // 2: core.message_broker.actors.v1.CloudEvent
}
var file_core_message_broker_actors_v1_producer_proto_depIdxs = []int32{
	2, // 0: core.message_broker.actors.v1.ProduceRequest.message:type_name -> core.message_broker.actors.v1.CloudEvent
	0, // 1: core.message_broker.actors.v1.Producer.Produce:input_type -> core.message_broker.actors.v1.ProduceRequest
	1, // 2: core.message_broker.actors.v1.Producer.Produce:output_type -> core.message_broker.actors.v1.ProduceResponse
	2, // [2:3] is the sub-list for method output_type
	1, // [1:2] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_core_message_broker_actors_v1_producer_proto_init() }
func file_core_message_broker_actors_v1_producer_proto_init() {
	if File_core_message_broker_actors_v1_producer_proto != nil {
		return
	}
	file_core_message_broker_actors_v1_models_proto_init()
	if !protoimpl.UnsafeEnabled {
		file_core_message_broker_actors_v1_producer_proto_msgTypes[0].Exporter = func(v any, i int) any {
			switch v := v.(*ProduceRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_core_message_broker_actors_v1_producer_proto_msgTypes[1].Exporter = func(v any, i int) any {
			switch v := v.(*ProduceResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_core_message_broker_actors_v1_producer_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_core_message_broker_actors_v1_producer_proto_goTypes,
		DependencyIndexes: file_core_message_broker_actors_v1_producer_proto_depIdxs,
		MessageInfos:      file_core_message_broker_actors_v1_producer_proto_msgTypes,
	}.Build()
	File_core_message_broker_actors_v1_producer_proto = out.File
	file_core_message_broker_actors_v1_producer_proto_rawDesc = nil
	file_core_message_broker_actors_v1_producer_proto_goTypes = nil
	file_core_message_broker_actors_v1_producer_proto_depIdxs = nil
}
