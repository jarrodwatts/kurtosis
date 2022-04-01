package docker

import (
	"bytes"
	"context"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_impls/docker/docker_manager"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_impls/docker/docker_manager/types"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_impls/docker/object_attributes_provider/label_key_consts"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_impls/docker/object_attributes_provider/label_value_consts"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/container_status"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/enclave"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/networking_sidecar"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/service"
	"github.com/kurtosis-tech/stacktrace"
	"github.com/sirupsen/logrus"
	"net"
	"strings"
)

const (
	networkingSidecarImageName = "kurtosistech/iproute2"
	succesfulExecCmdExitCode = 0
)

// We sleep forever because all the commands this container will run will be executed
//  via Docker exec
var sidecarContainerCommand = []string{
	"sleep", "infinity",
}

func (backendCore *DockerKurtosisBackend) CreateNetworkingSidecar(
	ctx context.Context,
	enclaveId enclave.EnclaveID,
	serviceGuid service.ServiceGUID,
	ipAddr net.IP, // TODO REMOVE THIS ONCE WE FIX THE STATIC IP PROBLEM!!
)(
	*networking_sidecar.NetworkingSidecar,
	error,
){
	// Get the Docker network ID where we'll start the new sidecar container
	enclaveNetwork, err := backendCore.getEnclaveNetworkByEnclaveId(ctx, enclaveId)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting enclave network by enclave ID '%v'", enclaveId)
	}

	enclaveStatus, enclaveContainers, err := backendCore.getEnclaveStatusAndContainers(ctx, enclaveId)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting enclave status and containers for enclave with ID '%v'", enclaveId)
	}

	if enclaveStatus != enclave.EnclaveStatus_Running {
		return nil, stacktrace.NewError("Networking sidecar for user service with GUID '%v' can not be created inside enclave with ID '%v' because its current status is '%v' and it must be '%v' to accept new nodes", serviceGuid, enclaveId, enclaveStatus, enclave.EnclaveStatus_Running.String())
	}

	userServiceContainer, err := getUserServiceContainerFromContainerListByEnclaveIdAndUserServiceGUID(enclaveContainers, enclaveId, serviceGuid)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting user service container with GUID '%v' from container list '%+v' which are part of enclave with ID '%v'", serviceGuid, enclaveContainers, enclaveId)
	}

	enclaveObjAttrsProvider, err := backendCore.objAttrsProvider.ForEnclave(enclaveId)
	if err != nil {
		return nil, stacktrace.Propagate(err, "Couldn't get an object attribute provider for enclave '%v'", enclaveId)
	}

	containerAttrs, err := enclaveObjAttrsProvider.ForNetworkingSidecarContainer(serviceGuid, ipAddr)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred while trying to get the networking sidecar container attributes for user service with GUID '%v'", serviceGuid)
	}
	containerName := containerAttrs.GetName()
	containerDockerLabels := containerAttrs.GetLabels()

	containerLabels := map[string]string{}
	for dockerLabelKey, dockerLabelValue := range containerDockerLabels {
		containerLabels[dockerLabelKey.GetString()] = dockerLabelValue.GetString()
	}
	createAndStartArgs := docker_manager.NewCreateAndStartContainerArgsBuilder(
		networkingSidecarImageName,
		containerName.GetString(),
		enclaveNetwork.GetId(),
	).WithAlias(
		containerName.GetString(),
	).WithStaticIP(
		ipAddr,
	).WithAddedCapabilities(map[docker_manager.ContainerCapability]bool{
		docker_manager.NetAdmin: true,
	}).WithNetworkMode(
		docker_manager.NewContainerNetworkMode(userServiceContainer.GetId()),
	).WithCmdArgs(
		sidecarContainerCommand,
	).WithLabels(
		containerLabels,
	).Build()

	// Best-effort pull attempt
	if err = backendCore.dockerManager.PullImage(ctx, networkingSidecarImageName); err != nil {
		logrus.Warnf("Failed to pull the latest version of networking sidecar container image '%v'; you may be running an out-of-date version", networkingSidecarImageName)
	}

	_, _, err = backendCore.dockerManager.CreateAndStartContainer(ctx, createAndStartArgs)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred starting the networking sidecar container")
	}

	networkingSidecar, err := getNetworkingSidecarObjectFromContainerInfo(containerLabels, types.ContainerStatus_Running)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting networking sidecar object from container info with labels '%+v' and container status '%v'", containerLabels, types.ContainerStatus_Running)
	}

	return networkingSidecar, nil

}

func (backendCore *DockerKurtosisBackend) GetNetworkingSidecars(
	ctx context.Context,
	filters *networking_sidecar.NetworkingSidecarFilters,
) (
	map[service.ServiceGUID]*networking_sidecar.NetworkingSidecar,
	error,
) {

	networkingSidecars, err := backendCore.getMatchingNetworkingSidecars(ctx, filters)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting networking sidecars matching filters '%+v'", filters)
	}

	successfulNetworkingSidecars := map[service.ServiceGUID]*networking_sidecar.NetworkingSidecar{}
	for _, networkingSidecar := range networkingSidecars {
		successfulNetworkingSidecars[networkingSidecar.GetServiceGUID()] = networkingSidecar
	}

	return successfulNetworkingSidecars, nil
}

func (backendCore *DockerKurtosisBackend) RunNetworkingSidecarExecCommands(
	ctx context.Context,
	enclaveId enclave.EnclaveID,
	networkingSidecarsCommands map[service.ServiceGUID][]string,
)(
	map[service.ServiceGUID]bool,
	map[service.ServiceGUID]error,
	error,
){
	successfulUserServiceGuids := map[service.ServiceGUID]bool{}
	erroredUserServiceGuids := map[service.ServiceGUID]error{}

	userServiceGuids := map[service.ServiceGUID]bool{}
	for userServiceGuid := range networkingSidecarsCommands {
		userServiceGuids[userServiceGuid] = true
	}

	filters := &networking_sidecar.NetworkingSidecarFilters{
		EnclaveIDs: map[enclave.EnclaveID]bool{
			enclaveId: true,
		},
		UserServiceGUIDs: userServiceGuids,
	}

	networkingSidecars, err := backendCore.getMatchingNetworkingSidecars(ctx, filters)
	if err != nil {
		return nil, nil,  stacktrace.Propagate(err, "An error occurred getting networking sidecars matching filters '%+v'", filters)
	}

	// TODO Parallelize to increase perf
	for containerId, networkingSidecar := range networkingSidecars {
		networkingSidecarUnwrappedCommand := networkingSidecarsCommands[networkingSidecar.GetServiceGUID()]

		networkingSidecarShWrappedCmd := wrapNetworkingSidecarContainerShCommand(networkingSidecarUnwrappedCommand)

		execOutputBuf := &bytes.Buffer{}
		exitCode, err := backendCore.dockerManager.RunExecCommand(
			ctx,
			containerId,
			networkingSidecarShWrappedCmd,
			execOutputBuf)
		if err != nil {
			wrappedErr := stacktrace.Propagate(
				err,
				"An error occurred executing command '%+v' on networking sidecar with user service GUID '%v'",
				networkingSidecarShWrappedCmd,
				containerId,
			)
			erroredUserServiceGuids[networkingSidecar.GetServiceGUID()] = wrappedErr
			continue
		}
		if exitCode != succesfulExecCmdExitCode {
			exitCodeErr := stacktrace.NewError(
				"Expected exit code '%v' when running exec command '%+v' on networking sidecar with user service GUID '%v', but got exit code '%v' instead with the following output:\n%v",
				succesfulExecCmdExitCode,
				networkingSidecarShWrappedCmd,
				containerId,
				exitCode,
				execOutputBuf.String(),
			)
			erroredUserServiceGuids[networkingSidecar.GetServiceGUID()] = exitCodeErr
			continue
		}
		successfulUserServiceGuids[networkingSidecar.GetServiceGUID()] = true
	}

	return successfulUserServiceGuids, erroredUserServiceGuids, nil
}

func (backendCore *DockerKurtosisBackend) StopNetworkingSidecars(
	ctx context.Context,
	filters *networking_sidecar.NetworkingSidecarFilters,
) (
	map[service.ServiceGUID]bool,
	map[service.ServiceGUID]error,
	error,
) {

	successfulUserServiceGuids := map[service.ServiceGUID]bool{}
	erroredUserServiceGuids := map[service.ServiceGUID]error{}

	networkingSidecars, err := backendCore.getMatchingNetworkingSidecars(ctx, filters)
	if err != nil {
		return nil, nil,  stacktrace.Propagate(err, "An error occurred getting networking sidecars matching filters '%+v'", filters)
	}

	for containerId, networkingSidecar := range networkingSidecars {
		if err := backendCore.killContainerAndWaitForExit(ctx, containerId); err != nil {
			wrappedErr := stacktrace.Propagate(err, "An error occurred killing networking sidecar container with user service GUID '%v' and container ID '%v'", networkingSidecar.GetServiceGUID(), containerId)
			erroredUserServiceGuids[networkingSidecar.GetServiceGUID()] = wrappedErr
			continue
		}
		successfulUserServiceGuids[networkingSidecar.GetServiceGUID()] = true
	}

	return successfulUserServiceGuids, erroredUserServiceGuids, nil
}

func (backendCore *DockerKurtosisBackend) DestroyNetworkingSidecars(
	ctx context.Context,
	filters *networking_sidecar.NetworkingSidecarFilters,
) (
	map[service.ServiceGUID]bool,
	map[service.ServiceGUID]error,
	error,
) {
	successfulUserServiceGuids := map[service.ServiceGUID]bool{}
	erroredUserServiceGuids := map[service.ServiceGUID]error{}

	networkingSidecars, err := backendCore.getMatchingNetworkingSidecars(ctx, filters)
	if err != nil {
		return nil, nil,  stacktrace.Propagate(err, "An error occurred getting networking sidecars matching filters '%+v'", filters)
	}

	for containerId, networkingSidecar := range networkingSidecars {
		if err := backendCore.dockerManager.RemoveContainer(ctx, containerId); err != nil {
			wrappedErr := stacktrace.Propagate(
				err,
				"An error occurred removing container with ID '%v'",
				containerId,
			)
			erroredUserServiceGuids[networkingSidecar.GetServiceGUID()] = wrappedErr
			continue
		}
		successfulUserServiceGuids[networkingSidecar.GetServiceGUID()] = true
	}
	return successfulUserServiceGuids, erroredUserServiceGuids, nil
}

// ====================================================================================================
// 									   Private helper methods
// ====================================================================================================
// Embeds the given command in a call to sh shell, so that a command with things
//  like '&&' will get executed as expected
func wrapNetworkingSidecarContainerShCommand(unwrappedCmd []string) []string {
	return []string{
		"sh",
		"-c",
		strings.Join(unwrappedCmd, " "),
	}
}

func (backendCore *DockerKurtosisBackend) getMatchingNetworkingSidecars(
	ctx context.Context,
	filters *networking_sidecar.NetworkingSidecarFilters,
) (map[string]*networking_sidecar.NetworkingSidecar, error) {

	searchLabels := map[string]string{
		label_key_consts.AppIDLabelKey.GetString():         label_value_consts.AppIDLabelValue.GetString(),
		label_key_consts.ContainerTypeLabelKey.GetString(): label_value_consts.NetworkingSidecarContainerTypeLabelValue.GetString(),
	}
	matchingContainers, err := backendCore.dockerManager.GetContainersByLabels(ctx, searchLabels, shouldFetchAllContainersWhenRetrievingContainers)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred fetching containers using labels: %+v", searchLabels)
	}

	matchingObjects := map[string]*networking_sidecar.NetworkingSidecar{}
	for _, container := range matchingContainers {
		containerId := container.GetId()
		object, err := getNetworkingSidecarObjectFromContainerInfo(
			container.GetLabels(),
			container.GetStatus(),
		)
		if err != nil {
			return nil, stacktrace.Propagate(err, "An error occurred converting container with ID '%v' into a networking sidecar object", container.GetId())
		}

		if filters.EnclaveIDs != nil && len(filters.EnclaveIDs) > 0 {
			if _, found := filters.EnclaveIDs[object.GetEnclaveID()]; !found {
				continue
			}
		}

		if filters.UserServiceGUIDs != nil && len(filters.UserServiceGUIDs) > 0 {
			if _, found := filters.UserServiceGUIDs[object.GetServiceGUID()]; !found {
				continue
			}
		}

		if filters.Statuses != nil && len(filters.Statuses) > 0 {
			if _, found := filters.Statuses[object.GetStatus()]; !found {
				continue
			}
		}

		matchingObjects[containerId] = object
	}

	return matchingObjects, nil
}

func getNetworkingSidecarObjectFromContainerInfo(
	labels map[string]string,
	containerStatus types.ContainerStatus,
) (*networking_sidecar.NetworkingSidecar, error) {

	enclaveId, found := labels[label_key_consts.EnclaveIDLabelKey.GetString()]
	if !found {
		return nil, stacktrace.NewError("Expected the networking sidecar's enclave ID to be found under label '%v' but the label wasn't present", label_key_consts.EnclaveIDLabelKey.GetString())
	}

	guid, found := labels[label_key_consts.GUIDLabelKey.GetString()]
	if !found {
		return nil, stacktrace.NewError("Expected to find user service GUID label key '%v' but none was found", label_key_consts.GUIDLabelKey.GetString())
	}

	isContainerRunning, found := isContainerRunningDeterminer[containerStatus]
	if !found {
		// This should never happen because we enforce completeness in a unit test
		return nil, stacktrace.NewError("No is-running designation found for networking sidecar container status '%v'; this is a bug in Kurtosis!", containerStatus.String())
	}
	var status container_status.ContainerStatus
	if isContainerRunning {
		status = container_status.ContainerStatus_Running
	} else {
		status = container_status.ContainerStatus_Stopped
	}

	var privateIpAddr net.IP
	privateIpAddrStr, found := labels[label_key_consts.PrivateIPLabelKey.GetString()]
	// UNCOMMENT THIS AFTER 2022-06-30 WHEN NOBODY HAS NETWORKING SIDECAR WITHOUT THE PRIVATE IP ADDRESS LABEL
	/*
		if !found {
			return nil, stacktrace.NewError("Expected to find networking sidecar private IP label key '%v' but none was found", label_key_consts.PrivateIPLabelKey.GetString())
		}
	*/
	if found {
		candidatePrivateIpAddr := net.ParseIP(privateIpAddrStr)
		if candidatePrivateIpAddr == nil {
			return nil, stacktrace.NewError("Couldn't parse private IP address string '%v' to an IP", privateIpAddrStr)
		}
		privateIpAddr = candidatePrivateIpAddr
	}

	newObject := networking_sidecar.NewNetworkingSidecar(
		service.ServiceGUID(guid),
		privateIpAddr,
		enclave.EnclaveID(enclaveId),
		status,
	)

	return newObject, nil
}