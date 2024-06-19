// @generated by protoc-gen-connect-query v1.1.3 with parameter "target=js"
// @generated from file examples/crud/v1/service.proto (package examples.crud.v1, syntax proto3)
/* eslint-disable */
// @ts-nocheck

import { MethodKind } from "@bufbuild/protobuf";
import { CreateRequest, CreateResponse, DeleteRequest, DeleteResponse, ReadRequest, ReadResponse, UpdateRequest, UpdateResponse } from "./service_pb.js";

/**
 * @generated from rpc examples.crud.v1.CrudService.Create
 */
export const create = {
  localName: "create",
  name: "Create",
  kind: MethodKind.Unary,
  I: CreateRequest,
  O: CreateResponse,
  service: {
    typeName: "examples.crud.v1.CrudService"
  }
};

/**
 * @generated from rpc examples.crud.v1.CrudService.Read
 */
export const read = {
  localName: "read",
  name: "Read",
  kind: MethodKind.Unary,
  I: ReadRequest,
  O: ReadResponse,
  service: {
    typeName: "examples.crud.v1.CrudService"
  }
};

/**
 * @generated from rpc examples.crud.v1.CrudService.Update
 */
export const update = {
  localName: "update",
  name: "Update",
  kind: MethodKind.Unary,
  I: UpdateRequest,
  O: UpdateResponse,
  service: {
    typeName: "examples.crud.v1.CrudService"
  }
};

/**
 * @generated from rpc examples.crud.v1.CrudService.Delete
 */
export const delete$ = {
  localName: "delete",
  name: "Delete",
  kind: MethodKind.Unary,
  I: DeleteRequest,
  O: DeleteResponse,
  service: {
    typeName: "examples.crud.v1.CrudService"
  }
};
