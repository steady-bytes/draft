// @generated by protoc-gen-es v1.6.0 with parameter "target=js"
// @generated from file examples/echo/v1/service.proto (package examples.echo.v1, syntax proto3)
/* eslint-disable */
// @ts-nocheck

import { proto3 } from "@bufbuild/protobuf";

/**
 * @generated from message examples.echo.v1.SpeakRequest
 */
export const SpeakRequest = proto3.makeMessageType(
  "examples.echo.v1.SpeakRequest",
  () => [
    { no: 1, name: "input", kind: "scalar", T: 9 /* ScalarType.STRING */ },
  ],
);

/**
 * @generated from message examples.echo.v1.SpeakResponse
 */
export const SpeakResponse = proto3.makeMessageType(
  "examples.echo.v1.SpeakResponse",
  () => [
    { no: 2, name: "output", kind: "scalar", T: 9 /* ScalarType.STRING */ },
  ],
);

