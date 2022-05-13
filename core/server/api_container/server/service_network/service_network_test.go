/*
 * Copyright (c) 2021 - present Kurtosis Technologies Inc.
 * All Rights Reserved.
 */

package service_network

import (
	"context"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/user_service_registration"
	"github.com/kurtosis-tech/kurtosis-core/server/api_container/server/service_network/networking_sidecar"
	"github.com/stretchr/testify/require"
	"net"
	"strconv"
	"testing"
)

const (
	packetLossConfigForBlockedPartition = float32(100)
)

func TestUpdateTrafficControl(t *testing.T) {
	numServices := 10
	ctx := context.Background()

	sidecars := map[user_service_registration.ServiceID]networking_sidecar.NetworkingSidecarWrapper{}
	mockSidecars := map[user_service_registration.ServiceID]*networking_sidecar.MockNetworkingSidecarWrapper{}
	for i := 0; i < numServices; i++ {
		serviceId := testServiceIdFromInt(i)
		sidecar := networking_sidecar.NewMockNetworkingSidecarWrapper()
		sidecars[serviceId] = sidecar
		mockSidecars[serviceId] = sidecar
	}

	registrationInfo := map[user_service_registration.ServiceID]serviceRegistrationInfo{}
	for i := 0; i < numServices; i++ {
		serviceId := testServiceIdFromInt(i)
		serviceGUID := newServiceGUID(serviceId)
		ip := testIpFromInt(i)
		registrationInfo[serviceId] = serviceRegistrationInfo{serviceGUID: serviceGUID, privateIpAddr: ip}
	}

	// Creates the pathological "line" of connections, where each service can only see the services adjacent
	targetServicePacketLossConfigs := map[user_service_registration.ServiceID]map[user_service_registration.ServiceID]float32{}
	for i := 0; i < numServices; i++ {
		serviceId := testServiceIdFromInt(i)
		otherServicesPacketLossConfig := map[user_service_registration.ServiceID]float32{}
		for j := 0; j < numServices; j++ {
			if j < i-1 || j > i+1 {
				blockedServiceId := testServiceIdFromInt(j)
				otherServicesPacketLossConfig[blockedServiceId] = packetLossConfigForBlockedPartition
			}
		}
		targetServicePacketLossConfigs[serviceId] = otherServicesPacketLossConfig
	}

	require.Nil(t, updateTrafficControlConfiguration(ctx, targetServicePacketLossConfigs, registrationInfo, sidecars))

	// Verify that each service got told to block exactly the right things
	for i := 0; i < numServices; i++ {
		serviceId := testServiceIdFromInt(i)

		expected := map[string]float32{}
		for j := 0; j < numServices; j++ {
			if j < i-1 || j > i+1 {
				ip := testIpFromInt(j)
				expected[ip.String()] = packetLossConfigForBlockedPartition
			}
		}

		mockSidecar := mockSidecars[serviceId]
		recordedPacketLossConfig := mockSidecar.GetRecordedUpdatePacketLossConfig()
		require.Equal(t, 1, len(recordedPacketLossConfig), "Expected sidecar for service ID '%v' to have recorded exactly one call to update")

		actualPacketLossConfigForService := recordedPacketLossConfig[0]

		require.Equal(t, expected, actualPacketLossConfigForService)
	}
}

func testIpFromInt(i int) net.IP {
	return []byte{1, 1, 1, byte(i)}
}

func testServiceIdFromInt(i int) user_service_registration.ServiceID {
	return user_service_registration.ServiceID("service-" + strconv.Itoa(i))
}