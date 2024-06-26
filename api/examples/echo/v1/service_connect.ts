// @generated by protoc-gen-connect-es v1.3.0 with parameter "target=ts"
// @generated from file examples/echo/v1/service.proto (package examples.echo.v1, syntax proto3)
/* eslint-disable */
// @ts-nocheck

import { SpeakRequest, SpeakResponse } from "./service_pb.js";
import { MethodKind } from "@bufbuild/protobuf";

/**
 * @generated from service examples.echo.v1.EchoService
 */
export const EchoService = {
  typeName: "examples.echo.v1.EchoService",
  methods: {
    /**
     * @generated from rpc examples.echo.v1.EchoService.Speak
     */
    speak: {
      name: "Speak",
      I: SpeakRequest,
      O: SpeakResponse,
      kind: MethodKind.Unary,
    },
  }
} as const;

