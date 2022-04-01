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

var gorm_options_pb = require('./gorm/options_pb.js')
const proto = {};
proto.api = require('./draft_pb.js');

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


/**
 * @param {string} hostname
 * @param {?Object} credentials
 * @param {?grpc.web.ClientOptions} options
 * @constructor
 * @struct
 * @final
 */
proto.api.RegistryClient =
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
proto.api.RegistryPromiseClient =
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
 *   !proto.api.JoinRequest,
 *   !proto.api.JoinResponse>}
 */
const methodDescriptor_Registry_Join = new grpc.web.MethodDescriptor(
  '/api.Registry/Join',
  grpc.web.MethodType.UNARY,
  proto.api.JoinRequest,
  proto.api.JoinResponse,
  /**
   * @param {!proto.api.JoinRequest} request
   * @return {!Uint8Array}
   */
  function(request) {
    return request.serializeBinary();
  },
  proto.api.JoinResponse.deserializeBinary
);


/**
 * @param {!proto.api.JoinRequest} request The
 *     request proto
 * @param {?Object<string, string>} metadata User defined
 *     call metadata
 * @param {function(?grpc.web.RpcError, ?proto.api.JoinResponse)}
 *     callback The callback function(error, response)
 * @return {!grpc.web.ClientReadableStream<!proto.api.JoinResponse>|undefined}
 *     The XHR Node Readable Stream
 */
proto.api.RegistryClient.prototype.join =
    function(request, metadata, callback) {
  return this.client_.rpcCall(this.hostname_ +
      '/api.Registry/Join',
      request,
      metadata || {},
      methodDescriptor_Registry_Join,
      callback);
};


/**
 * @param {!proto.api.JoinRequest} request The
 *     request proto
 * @param {?Object<string, string>=} metadata User defined
 *     call metadata
 * @return {!Promise<!proto.api.JoinResponse>}
 *     Promise that resolves to the response
 */
proto.api.RegistryPromiseClient.prototype.join =
    function(request, metadata) {
  return this.client_.unaryCall(this.hostname_ +
      '/api.Registry/Join',
      request,
      metadata || {},
      methodDescriptor_Registry_Join);
};


/**
 * @const
 * @type {!grpc.web.MethodDescriptor<
 *   !proto.api.LeaveRequest,
 *   !proto.api.LeaveResponse>}
 */
const methodDescriptor_Registry_Leave = new grpc.web.MethodDescriptor(
  '/api.Registry/Leave',
  grpc.web.MethodType.UNARY,
  proto.api.LeaveRequest,
  proto.api.LeaveResponse,
  /**
   * @param {!proto.api.LeaveRequest} request
   * @return {!Uint8Array}
   */
  function(request) {
    return request.serializeBinary();
  },
  proto.api.LeaveResponse.deserializeBinary
);


/**
 * @param {!proto.api.LeaveRequest} request The
 *     request proto
 * @param {?Object<string, string>} metadata User defined
 *     call metadata
 * @param {function(?grpc.web.RpcError, ?proto.api.LeaveResponse)}
 *     callback The callback function(error, response)
 * @return {!grpc.web.ClientReadableStream<!proto.api.LeaveResponse>|undefined}
 *     The XHR Node Readable Stream
 */
proto.api.RegistryClient.prototype.leave =
    function(request, metadata, callback) {
  return this.client_.rpcCall(this.hostname_ +
      '/api.Registry/Leave',
      request,
      metadata || {},
      methodDescriptor_Registry_Leave,
      callback);
};


/**
 * @param {!proto.api.LeaveRequest} request The
 *     request proto
 * @param {?Object<string, string>=} metadata User defined
 *     call metadata
 * @return {!Promise<!proto.api.LeaveResponse>}
 *     Promise that resolves to the response
 */
proto.api.RegistryPromiseClient.prototype.leave =
    function(request, metadata) {
  return this.client_.unaryCall(this.hostname_ +
      '/api.Registry/Leave',
      request,
      metadata || {},
      methodDescriptor_Registry_Leave);
};


/**
 * @param {string} hostname
 * @param {?Object} credentials
 * @param {?grpc.web.ClientOptions} options
 * @constructor
 * @struct
 * @final
 */
proto.api.EventStoreClient =
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
proto.api.EventStorePromiseClient =
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
 *   !proto.api.CreateEventRequest,
 *   !proto.api.CreateEventResponse>}
 */
const methodDescriptor_EventStore_Create = new grpc.web.MethodDescriptor(
  '/api.EventStore/Create',
  grpc.web.MethodType.UNARY,
  proto.api.CreateEventRequest,
  proto.api.CreateEventResponse,
  /**
   * @param {!proto.api.CreateEventRequest} request
   * @return {!Uint8Array}
   */
  function(request) {
    return request.serializeBinary();
  },
  proto.api.CreateEventResponse.deserializeBinary
);


/**
 * @param {!proto.api.CreateEventRequest} request The
 *     request proto
 * @param {?Object<string, string>} metadata User defined
 *     call metadata
 * @param {function(?grpc.web.RpcError, ?proto.api.CreateEventResponse)}
 *     callback The callback function(error, response)
 * @return {!grpc.web.ClientReadableStream<!proto.api.CreateEventResponse>|undefined}
 *     The XHR Node Readable Stream
 */
proto.api.EventStoreClient.prototype.create =
    function(request, metadata, callback) {
  return this.client_.rpcCall(this.hostname_ +
      '/api.EventStore/Create',
      request,
      metadata || {},
      methodDescriptor_EventStore_Create,
      callback);
};


/**
 * @param {!proto.api.CreateEventRequest} request The
 *     request proto
 * @param {?Object<string, string>=} metadata User defined
 *     call metadata
 * @return {!Promise<!proto.api.CreateEventResponse>}
 *     Promise that resolves to the response
 */
proto.api.EventStorePromiseClient.prototype.create =
    function(request, metadata) {
  return this.client_.unaryCall(this.hostname_ +
      '/api.EventStore/Create',
      request,
      metadata || {},
      methodDescriptor_EventStore_Create);
};


module.exports = proto.api;

