// @generated by protoc-gen-connect-es v0.12.0 with parameter "target=js+dts"
// @generated from file portal_daemon_service.proto (package portal_daemon_api, syntax proto3)
/* eslint-disable */
// @ts-nocheck

import { ForwardUserServicePortArgs, ForwardUserServicePortResponse, PortalPing, PortalPong } from "./portal_daemon_service_pb.js";
import { MethodKind } from "@bufbuild/protobuf";

/**
 * @generated from service portal_daemon_api.KurtosisPortalDaemon
 */
export declare const KurtosisPortalDaemon: {
  readonly typeName: "portal_daemon_api.KurtosisPortalDaemon",
  readonly methods: {
    /**
     * To check availability
     *
     * @generated from rpc portal_daemon_api.KurtosisPortalDaemon.Ping
     */
    readonly ping: {
      readonly name: "Ping",
      readonly I: typeof PortalPing,
      readonly O: typeof PortalPong,
      readonly kind: MethodKind.Unary,
    },
    /**
     * @generated from rpc portal_daemon_api.KurtosisPortalDaemon.ForwardUserServicePort
     */
    readonly forwardUserServicePort: {
      readonly name: "ForwardUserServicePort",
      readonly I: typeof ForwardUserServicePortArgs,
      readonly O: typeof ForwardUserServicePortResponse,
      readonly kind: MethodKind.Unary,
    },
  }
};
