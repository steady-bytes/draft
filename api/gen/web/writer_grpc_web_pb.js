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
proto.api = require('./writer_pb.js');

/**
 * @param {string} hostname
 * @param {?Object} credentials
 * @param {?grpc.web.ClientOptions} options
 * @constructor
 * @struct
 * @final
 */
proto.api.WriterClient =
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
proto.api.WriterPromiseClient =
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
 *   !proto.api.Command,
 *   !proto.api.Output>}
 */
const methodDescriptor_Writer_Exec = new grpc.web.MethodDescriptor(
  '/api.Writer/Exec',
  grpc.web.MethodType.UNARY,
  proto.api.Command,
  proto.api.Output,
  /**
   * @param {!proto.api.Command} request
   * @return {!Uint8Array}
   */
  function(request) {
    return request.serializeBinary();
  },
  proto.api.Output.deserializeBinary
);


/**
 * @param {!proto.api.Command} request The
 *     request proto
 * @param {?Object<string, string>} metadata User defined
 *     call metadata
 * @param {function(?grpc.web.RpcError, ?proto.api.Output)}
 *     callback The callback function(error, response)
 * @return {!grpc.web.ClientReadableStream<!proto.api.Output>|undefined}
 *     The XHR Node Readable Stream
 */
proto.api.WriterClient.prototype.exec =
    function(request, metadata, callback) {
  return this.client_.rpcCall(this.hostname_ +
      '/api.Writer/Exec',
      request,
      metadata || {},
      methodDescriptor_Writer_Exec,
      callback);
};


/**
 * @param {!proto.api.Command} request The
 *     request proto
 * @param {?Object<string, string>=} metadata User defined
 *     call metadata
 * @return {!Promise<!proto.api.Output>}
 *     Promise that resolves to the response
 */
proto.api.WriterPromiseClient.prototype.exec =
    function(request, metadata) {
  return this.client_.unaryCall(this.hostname_ +
      '/api.Writer/Exec',
      request,
      metadata || {},
      methodDescriptor_Writer_Exec);
};


/**
 * @const
 * @type {!grpc.web.MethodDescriptor<
 *   !proto.api.Command,
 *   !proto.api.Transaction>}
 */
const methodDescriptor_Writer_ExecSaga = new grpc.web.MethodDescriptor(
  '/api.Writer/ExecSaga',
  grpc.web.MethodType.UNARY,
  proto.api.Command,
  proto.api.Transaction,
  /**
   * @param {!proto.api.Command} request
   * @return {!Uint8Array}
   */
  function(request) {
    return request.serializeBinary();
  },
  proto.api.Transaction.deserializeBinary
);


/**
 * @param {!proto.api.Command} request The
 *     request proto
 * @param {?Object<string, string>} metadata User defined
 *     call metadata
 * @param {function(?grpc.web.RpcError, ?proto.api.Transaction)}
 *     callback The callback function(error, response)
 * @return {!grpc.web.ClientReadableStream<!proto.api.Transaction>|undefined}
 *     The XHR Node Readable Stream
 */
proto.api.WriterClient.prototype.execSaga =
    function(request, metadata, callback) {
  return this.client_.rpcCall(this.hostname_ +
      '/api.Writer/ExecSaga',
      request,
      metadata || {},
      methodDescriptor_Writer_ExecSaga,
      callback);
};


/**
 * @param {!proto.api.Command} request The
 *     request proto
 * @param {?Object<string, string>=} metadata User defined
 *     call metadata
 * @return {!Promise<!proto.api.Transaction>}
 *     Promise that resolves to the response
 */
proto.api.WriterPromiseClient.prototype.execSaga =
    function(request, metadata) {
  return this.client_.unaryCall(this.hostname_ +
      '/api.Writer/ExecSaga',
      request,
      metadata || {},
      methodDescriptor_Writer_ExecSaga);
};


module.exports = proto.api;

