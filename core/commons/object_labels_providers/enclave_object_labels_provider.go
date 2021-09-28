/*
 * Copyright (c) 2021 - present Kurtosis Technologies Inc.
 * All Rights Reserved.
 */

package object_labels_providers

import (
	"github.com/kurtosis-tech/kurtosis/api_container/server/lambda_store/lambda_store_types"
	"github.com/kurtosis-tech/kurtosis/api_container/server/service_network/service_network_types"
)

const (
	labelEnclaveIDKey     = "enclave-id"
	labelContainerTypeKey = "container-type"
	labelGUIDKey          = "guid"

	containerTypeApiContainer               = "api-container"
	containerTypeTestsuiteContainer         = "testsuite"
	containerTypeUserServiceContainer       = "user-service"
	containerTypeNetworkingSidecarContainer = "networking-sidecar"
	containerTypeLambdaContainer            = "lambda"
)

type EnclaveObjectLabelsProvider struct {
	enclaveId string
}

func NewEnclaveObjectLabelsProvider(enclaveId string) *EnclaveObjectLabelsProvider {
	return &EnclaveObjectLabelsProvider{enclaveId: enclaveId}
}

func (labelsProvider *EnclaveObjectLabelsProvider) ForApiContainer() map[string]string {
	labels := map[string]string{}
	labels[labelEnclaveIDKey] = labelsProvider.enclaveId
	labels[labelContainerTypeKey] = containerTypeApiContainer
	return labels
}

// TODO We don't want testsuites to be special - they should be Just Another Kurtosis Module - but we can't make them
//  unspecial (and thus delete this method) until the API container supports a container log-streaming endpoint
func (labelsProvider *EnclaveObjectLabelsProvider) ForTestRunningTestsuiteContainer() map[string]string {
	labels := map[string]string{}
	labels[labelEnclaveIDKey] = labelsProvider.enclaveId
	labels[labelContainerTypeKey] = containerTypeTestsuiteContainer
	return labels
}

func (labelsProvider *EnclaveObjectLabelsProvider) ForUserServiceContainer(serviceGUID service_network_types.ServiceGUID) map[string]string {
	labels := map[string]string{}
	labels[labelEnclaveIDKey] = labelsProvider.enclaveId
	labels[labelContainerTypeKey] = containerTypeUserServiceContainer
	labels[labelGUIDKey] = string(serviceGUID)
	return labels
}

func (labelsProvider *EnclaveObjectLabelsProvider) ForNetworkingSidecarContainer(serviceGUID service_network_types.ServiceGUID) map[string]string {
	labels := map[string]string{}
	labels[labelEnclaveIDKey] = labelsProvider.enclaveId
	labels[labelContainerTypeKey] = containerTypeNetworkingSidecarContainer
	labels[labelGUIDKey] = string(serviceGUID)
	return labels
}

func (labelsProvider *EnclaveObjectLabelsProvider) ForLambdaContainer(lambdaGUID lambda_store_types.LambdaGUID) map[string]string {
	labels := map[string]string{}
	labels[labelEnclaveIDKey] = labelsProvider.enclaveId
	labels[labelContainerTypeKey] = containerTypeLambdaContainer
	labels[labelGUIDKey] = string(lambdaGUID)
	return labels
}

func GetLabelsForListEnclaves() map[string]string {
	labels := map[string]string{}
	labels[labelContainerTypeKey] = containerTypeApiContainer
	return labels
}