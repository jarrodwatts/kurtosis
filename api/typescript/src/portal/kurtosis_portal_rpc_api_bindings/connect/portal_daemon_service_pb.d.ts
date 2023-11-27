// @generated by protoc-gen-es v1.3.0 with parameter "target=js+dts"
// @generated from file portal_daemon_service.proto (package portal_daemon_api, syntax proto3)
/* eslint-disable */
// @ts-nocheck

import type { BinaryReadOptions, FieldList, JsonReadOptions, JsonValue, PartialMessage, PlainMessage } from "@bufbuild/protobuf";
import { Message, proto3 } from "@bufbuild/protobuf";

/**
 * may specify a specific port, or an entire enclave's worth of services and ports
 *
 * @generated from message portal_daemon_api.EnclaveServicePortId
 */
export declare class EnclaveServicePortId extends Message<EnclaveServicePortId> {
  /**
   * @generated from field: string enclave_id = 1;
   */
  enclaveId: string;

  /**
   * @generated from field: optional string service_id = 2;
   */
  serviceId?: string;

  /**
   * @generated from field: optional string port_id = 3;
   */
  portId?: string;

  constructor(data?: PartialMessage<EnclaveServicePortId>);

  static readonly runtime: typeof proto3;
  static readonly typeName = "portal_daemon_api.EnclaveServicePortId";
  static readonly fields: FieldList;

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): EnclaveServicePortId;

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): EnclaveServicePortId;

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): EnclaveServicePortId;

  static equals(a: EnclaveServicePortId | PlainMessage<EnclaveServicePortId> | undefined, b: EnclaveServicePortId | PlainMessage<EnclaveServicePortId> | undefined): boolean;
}

/**
 * @generated from message portal_daemon_api.ForwardedServicePortId
 */
export declare class ForwardedServicePortId extends Message<ForwardedServicePortId> {
  /**
   * @generated from field: portal_daemon_api.EnclaveServicePortId enclaveServicePortId = 1;
   */
  enclaveServicePortId?: EnclaveServicePortId;

  /**
   * @generated from field: uint32 local_port_number = 2;
   */
  localPortNumber: number;

  constructor(data?: PartialMessage<ForwardedServicePortId>);

  static readonly runtime: typeof proto3;
  static readonly typeName = "portal_daemon_api.ForwardedServicePortId";
  static readonly fields: FieldList;

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): ForwardedServicePortId;

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): ForwardedServicePortId;

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): ForwardedServicePortId;

  static equals(a: ForwardedServicePortId | PlainMessage<ForwardedServicePortId> | undefined, b: ForwardedServicePortId | PlainMessage<ForwardedServicePortId> | undefined): boolean;
}

/**
 * @generated from message portal_daemon_api.CreateUserServicePortForwardArgs
 */
export declare class CreateUserServicePortForwardArgs extends Message<CreateUserServicePortForwardArgs> {
  /**
   * @generated from field: portal_daemon_api.EnclaveServicePortId enclaveServicePortId = 1;
   */
  enclaveServicePortId?: EnclaveServicePortId;

  /**
   * @generated from field: optional uint32 local_port_number = 2;
   */
  localPortNumber?: number;

  constructor(data?: PartialMessage<CreateUserServicePortForwardArgs>);

  static readonly runtime: typeof proto3;
  static readonly typeName = "portal_daemon_api.CreateUserServicePortForwardArgs";
  static readonly fields: FieldList;

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): CreateUserServicePortForwardArgs;

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): CreateUserServicePortForwardArgs;

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): CreateUserServicePortForwardArgs;

  static equals(a: CreateUserServicePortForwardArgs | PlainMessage<CreateUserServicePortForwardArgs> | undefined, b: CreateUserServicePortForwardArgs | PlainMessage<CreateUserServicePortForwardArgs> | undefined): boolean;
}

/**
 * @generated from message portal_daemon_api.CreateUserServicePortForwardResponse
 */
export declare class CreateUserServicePortForwardResponse extends Message<CreateUserServicePortForwardResponse> {
  /**
   * @generated from field: repeated portal_daemon_api.ForwardedServicePortId forwarded_port_numbers = 1;
   */
  forwardedPortNumbers: ForwardedServicePortId[];

  constructor(data?: PartialMessage<CreateUserServicePortForwardResponse>);

  static readonly runtime: typeof proto3;
  static readonly typeName = "portal_daemon_api.CreateUserServicePortForwardResponse";
  static readonly fields: FieldList;

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): CreateUserServicePortForwardResponse;

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): CreateUserServicePortForwardResponse;

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): CreateUserServicePortForwardResponse;

  static equals(a: CreateUserServicePortForwardResponse | PlainMessage<CreateUserServicePortForwardResponse> | undefined, b: CreateUserServicePortForwardResponse | PlainMessage<CreateUserServicePortForwardResponse> | undefined): boolean;
}

/**
 * @generated from message portal_daemon_api.RemoveUserServicePortForwardResponse
 */
export declare class RemoveUserServicePortForwardResponse extends Message<RemoveUserServicePortForwardResponse> {
  constructor(data?: PartialMessage<RemoveUserServicePortForwardResponse>);

  static readonly runtime: typeof proto3;
  static readonly typeName = "portal_daemon_api.RemoveUserServicePortForwardResponse";
  static readonly fields: FieldList;

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): RemoveUserServicePortForwardResponse;

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): RemoveUserServicePortForwardResponse;

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): RemoveUserServicePortForwardResponse;

  static equals(a: RemoveUserServicePortForwardResponse | PlainMessage<RemoveUserServicePortForwardResponse> | undefined, b: RemoveUserServicePortForwardResponse | PlainMessage<RemoveUserServicePortForwardResponse> | undefined): boolean;
}

/**
 * @generated from message portal_daemon_api.PortalPing
 */
export declare class PortalPing extends Message<PortalPing> {
  constructor(data?: PartialMessage<PortalPing>);

  static readonly runtime: typeof proto3;
  static readonly typeName = "portal_daemon_api.PortalPing";
  static readonly fields: FieldList;

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): PortalPing;

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): PortalPing;

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): PortalPing;

  static equals(a: PortalPing | PlainMessage<PortalPing> | undefined, b: PortalPing | PlainMessage<PortalPing> | undefined): boolean;
}

/**
 * @generated from message portal_daemon_api.PortalPong
 */
export declare class PortalPong extends Message<PortalPong> {
  constructor(data?: PartialMessage<PortalPong>);

  static readonly runtime: typeof proto3;
  static readonly typeName = "portal_daemon_api.PortalPong";
  static readonly fields: FieldList;

  static fromBinary(bytes: Uint8Array, options?: Partial<BinaryReadOptions>): PortalPong;

  static fromJson(jsonValue: JsonValue, options?: Partial<JsonReadOptions>): PortalPong;

  static fromJsonString(jsonString: string, options?: Partial<JsonReadOptions>): PortalPong;

  static equals(a: PortalPong | PlainMessage<PortalPong> | undefined, b: PortalPong | PlainMessage<PortalPong> | undefined): boolean;
}

