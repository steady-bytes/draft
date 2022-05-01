/**
 * @fileoverview gRPC-Web generated client stub for api
 * @enhanceable
 * @public
 */

// GENERATED CODE -- DO NOT EDIT!


/* eslint-disable */
// @ts-nocheck



const grpc = {};
grpc.web = require('grpc-web');


var google_protobuf_any_pb = require('google-protobuf/google/protobuf/any_pb.js')

var google_protobuf_timestamp_pb = require('google-protobuf/google/protobuf/timestamp_pb.js')

var validate_validate_pb = require('./validate/validate_pb.js')
const proto = {};
proto.api = require('./eventer_pb.js');

/**
 * @param {string} hostname
 * @param {?Object} credentials
 * @param {?grpc.web.ClientOptions} options
 * @constructor
 * @struct
 * @final
 */
proto.api.EventerClient =
    function(hostname, credentials, options) {
  if (!options) options = {};
  options.format = 'text';

  /**
   * @private @const {!grpc.web.GrpcWebClientBase} The client
   */
  this.client_ = new grpc.web.GrpcWebClientBase(options);

  /**
   * @private @const {string} The hostname
   */
  this.hostname_ = hostname;

};


/**
 * @param {string} hostname
 * @param {?Object} credentials
 * @param {?grpc.web.ClientOptions} options
 * @constructor
 * @struct
 * @final
 */
proto.api.EventerPromiseClient =
    function(hostname, credentials, options) {
  if (!options) options = {};
  options.format = 'text';

  /**
   * @private @const {!grpc.web.GrpcWebClientBase} The client
   */
  this.client_ = new grpc.web.GrpcWebClientBase(options);

  /**
   * @private @const {string} The hostname
   */
  this.hostname_ = hostname;

};


/**
 * @const
 * @type {!grpc.web.MethodDescriptor<
 *   !proto.api.EmitEventRequest,
 *   !proto.api.EmitEventResponse>}
 */
const methodDescriptor_Eventer_Emit = new grpc.web.MethodDescriptor(
  '/api.Eventer/Emit',
  grpc.web.MethodType.UNARY,
  proto.api.EmitEventRequest,
  proto.api.EmitEventResponse,
  /**
   * @param {!proto.api.EmitEventRequest} request
   * @return {!Uint8Array}
   */
  function(request) {
    return request.serializeBinary();
  },
  proto.api.EmitEventResponse.deserializeBinary
);


/**
 * @param {!proto.api.EmitEventRequest} request The
 *     request proto
 * @param {?Object<string, string>} metadata User defined
 *     call metadata
 * @param {function(?grpc.web.RpcError, ?proto.api.EmitEventResponse)}
 *     callback The callback function(error, response)
 * @return {!grpc.web.ClientReadableStream<!proto.api.EmitEventResponse>|undefined}
 *     The XHR Node Readable Stream
 */
proto.api.EventerClient.prototype.emit =
    function(request, metadata, callback) {
  return this.client_.rpcCall(this.hostname_ +
      '/api.Eventer/Emit',
      request,
      metadata || {},
      methodDescriptor_Eventer_Emit,
      callback);
};


/**
 * @param {!proto.api.EmitEventRequest} request The
 *     request proto
 * @param {?Object<string, string>=} metadata User defined
 *     call metadata
 * @return {!Promise<!proto.api.EmitEventResponse>}
 *     Promise that resolves to the response
 */
proto.api.EventerPromiseClient.prototype.emit =
    function(request, metadata) {
  return this.client_.unaryCall(this.hostname_ +
      '/api.Eventer/Emit',
      request,
      metadata || {},
      methodDescriptor_Eventer_Emit);
};


module.exports = proto.api;

