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
proto.api = require('./reader_pb.js');

/**
 * @param {string} hostname
 * @param {?Object} credentials
 * @param {?grpc.web.ClientOptions} options
 * @constructor
 * @struct
 * @final
 */
proto.api.ReaderClient =
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
proto.api.ReaderPromiseClient =
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
 *   !proto.api.ReadRequest,
 *   !proto.api.ReadResponse>}
 */
const methodDescriptor_Reader_Read = new grpc.web.MethodDescriptor(
  '/api.Reader/Read',
  grpc.web.MethodType.UNARY,
  proto.api.ReadRequest,
  proto.api.ReadResponse,
  /**
   * @param {!proto.api.ReadRequest} request
   * @return {!Uint8Array}
   */
  function(request) {
    return request.serializeBinary();
  },
  proto.api.ReadResponse.deserializeBinary
);


/**
 * @param {!proto.api.ReadRequest} request The
 *     request proto
 * @param {?Object<string, string>} metadata User defined
 *     call metadata
 * @param {function(?grpc.web.RpcError, ?proto.api.ReadResponse)}
 *     callback The callback function(error, response)
 * @return {!grpc.web.ClientReadableStream<!proto.api.ReadResponse>|undefined}
 *     The XHR Node Readable Stream
 */
proto.api.ReaderClient.prototype.read =
    function(request, metadata, callback) {
  return this.client_.rpcCall(this.hostname_ +
      '/api.Reader/Read',
      request,
      metadata || {},
      methodDescriptor_Reader_Read,
      callback);
};


/**
 * @param {!proto.api.ReadRequest} request The
 *     request proto
 * @param {?Object<string, string>=} metadata User defined
 *     call metadata
 * @return {!Promise<!proto.api.ReadResponse>}
 *     Promise that resolves to the response
 */
proto.api.ReaderPromiseClient.prototype.read =
    function(request, metadata) {
  return this.client_.unaryCall(this.hostname_ +
      '/api.Reader/Read',
      request,
      metadata || {},
      methodDescriptor_Reader_Read);
};


module.exports = proto.api;

