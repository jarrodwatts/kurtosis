/*
 * Copyright (c) 2021 - present Kurtosis Technologies Inc.
 * All Rights Reserved.
 */

package service_network

import (
	"context"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/files_artifact"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/port_spec"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/service"
	"github.com/kurtosis-tech/kurtosis-core/server/api_container/server/service_network/partition_topology"
	"github.com/kurtosis-tech/kurtosis-core/server/api_container/server/service_network/service_network_types"
	"net"
	"time"
)

type ServiceNetwork interface {
	/*
		Repartition
		Completely repartitions the network, throwing away the old topology
	*/
	Repartition(
		ctx context.Context,
		newPartitionServices map[service_network_types.PartitionID]map[service.ServiceID]bool,
		newPartitionConnections map[service_network_types.PartitionConnectionID]partition_topology.PartitionConnection,
		newDefaultConnection partition_topology.PartitionConnection,
	) error

	// RegisterService
	// Registers a service for use with the network (creating the IPs and so forth), but doesn't start it
	// If the partition ID is empty, registers the service with the default partition
	RegisterService(
		serviceId service.ServiceID,
		partitionId service_network_types.PartitionID,
	) (net.IP, string, error)

	/*
		StartService
		Starts a previously-registered but not-started service by creating it in a container

		Returns:
			resultPublicIpAddr: The public (outside-of-enclave) IP address where the service is reachable
			resultPublicPorts: Mapping of port_id -> public_port_where_private_port_is_reachable, where the port_id
				corresponds to the port_id passed into the privatePorts arg
	*/
	// TODO add tests for this
	StartService(
		ctx context.Context,
		serviceId service.ServiceID,
		imageName string,
		privatePorts map[string]*port_spec.PortSpec,
		entrypointArgs []string,
		cmdArgs []string,
		dockerEnvVars map[string]string,
		enclaveDataDirMountDirpath string,
		// TODO REMOVE
		oldFilesArtifactMountDirpaths map[files_artifact.FilesArtifactID]string,
		filesArtifactMountDirpaths map[files_artifact.FilesArtifactID]string,
	) (
		resultPublicIpAddr net.IP,
		resultPublicPorts map[string]*port_spec.PortSpec,
		resultErr error,
	)

	RemoveService(
		ctx context.Context,
		serviceId service.ServiceID,
		containerStopTimeout time.Duration,
	) error

	ExecCommand(
		ctx context.Context,
		serviceId service.ServiceID,
		command []string,
	) (int32, string, error)

	GetServiceRegistrationInfo(serviceId service.ServiceID) (
		privateIpAddr net.IP,
		relativeServiceDirpath string, // The dirpath, relative to the enclave data dir, where the service directory lives
		resultErr error,
	)

	GetServiceRunInfo(serviceId service.ServiceID) (
		privatePorts map[string]*port_spec.PortSpec,
		maybePublicIpAddr net.IP, // Will be nil if the service didn't declare any private ports
		publicPorts map[string]*port_spec.PortSpec, // Will be empty if the service didn't declare any private ports
		enclaveDataDirMntDirpath string, // The filepath on the service container where the enclave data dir is mounted
		resultErr error,
	)

	GetServiceIDs() map[service.ServiceID]bool
}
