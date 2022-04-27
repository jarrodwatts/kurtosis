/*
 * Copyright (c) 2021 - present Kurtosis Technologies Inc.
 * All Rights Reserved.
 */

package server

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/module"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/port_spec"
	kurtosis_backend_service "github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/service"
	"github.com/kurtosis-tech/kurtosis-core/api/golang/kurtosis_core_rpc_api_bindings"
	"github.com/kurtosis-tech/kurtosis-core/api/golang/lib/binding_constructors"
	"github.com/kurtosis-tech/kurtosis-core/server/api_container/server/module_store"
	"github.com/kurtosis-tech/kurtosis-core/server/api_container/server/service_network"
	"github.com/kurtosis-tech/kurtosis-core/server/api_container/server/service_network/partition_topology"
	"github.com/kurtosis-tech/kurtosis-core/server/api_container/server/service_network/service_network_types"
	"github.com/kurtosis-tech/kurtosis-core/server/commons/enclave_data_directory"
	"github.com/kurtosis-tech/metrics-library/golang/lib/client"
	"github.com/kurtosis-tech/stacktrace"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/types/known/emptypb"
	"io"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

const (
	// Custom-set max size for logs coming back from docker exec.
	// Protobuf sets a maximum of 2GB for responses, in interest of keeping performance sane
	// we pick a reasonable limit of 10MB on log responses for docker exec.
	// See: https://stackoverflow.com/questions/34128872/google-protobuf-maximum-size/34186672
	maxLogOutputSizeBytes = 10 * 1024 * 1024

	// The string returned by the API if a service's public IP address doesn't exist
	missingPublicIpAddrStr = ""

	TempDirFileFileMode os.FileMode = 0744
)

// Guaranteed (by a unit test) to be a 1:1 mapping between API port protos and port spec protos
var apiContainerPortProtoToPortSpecPortProto = map[kurtosis_core_rpc_api_bindings.Port_Protocol]port_spec.PortProtocol{
	kurtosis_core_rpc_api_bindings.Port_TCP:  port_spec.PortProtocol_TCP,
	kurtosis_core_rpc_api_bindings.Port_SCTP: port_spec.PortProtocol_SCTP,
	kurtosis_core_rpc_api_bindings.Port_UDP:  port_spec.PortProtocol_UDP,
}

type ApiContainerService struct {
	// This embedding is required by gRPC
	kurtosis_core_rpc_api_bindings.UnimplementedApiContainerServiceServer

	enclaveDataDir *enclave_data_directory.EnclaveDataDirectory

	serviceNetwork service_network.ServiceNetwork

	moduleStore *module_store.ModuleStore

	metricsClient client.MetricsClient
}

func NewApiContainerService(
	enclaveDirectory *enclave_data_directory.EnclaveDataDirectory,
	serviceNetwork service_network.ServiceNetwork,
	moduleStore *module_store.ModuleStore,
	metricsClient client.MetricsClient,
) (*ApiContainerService, error) {
	service := &ApiContainerService{
		enclaveDataDir:         enclaveDirectory,
		serviceNetwork:         serviceNetwork,
		moduleStore:            moduleStore,
		metricsClient:          metricsClient,
	}

	return service, nil
}

func (apicService ApiContainerService) LoadModule(ctx context.Context, args *kurtosis_core_rpc_api_bindings.LoadModuleArgs) (*kurtosis_core_rpc_api_bindings.LoadModuleResponse, error) {
	moduleId := module.ModuleID(args.ModuleId)
	image := args.ContainerImage
	serializedParams := args.SerializedParams

	if err := apicService.metricsClient.TrackLoadModule(args.ModuleId, image, serializedParams); err != nil {
		//We don't want to interrupt users flow if something fails when tracking metrics
		logrus.Errorf("An error occurred tracking load module event\n%v", err)
	}

	loadedModule, err := apicService.moduleStore.LoadModule(ctx, moduleId, image, serializedParams)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred loading module '%v' with container image '%v' and serialized params '%v'", moduleId, image, serializedParams)
	}
	privateApiPort, err := transformPortSpecToApiPort(loadedModule.GetPrivatePort())
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred transforming the module's private port spec port to an API port")
	}
	publicApiPort, err := transformPortSpecToApiPort(loadedModule.GetPublicPort())
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred transforming the module's public port spec port to an API port")
	}

	result := binding_constructors.NewLoadModuleResponse(
		string(loadedModule.GetGUID()),
		loadedModule.GetPrivateIp().String(),
		privateApiPort,
		loadedModule.GetPublicIp().String(),
		publicApiPort,
	)
	return result, nil
}

func (apicService ApiContainerService) UnloadModule(ctx context.Context, args *kurtosis_core_rpc_api_bindings.UnloadModuleArgs) (*emptypb.Empty, error) {
	moduleId := module.ModuleID(args.ModuleId)

	if err := apicService.metricsClient.TrackUnloadModule(args.ModuleId); err != nil {
		//We don't want to interrupt users flow if something fails when tracking metrics
		logrus.Errorf("An error occurred tracking unload module event\n%v", err)
	}

	if err := apicService.moduleStore.UnloadModule(ctx, moduleId); err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred unloading module '%v' from the network", moduleId)
	}

	return &emptypb.Empty{}, nil
}

func (apicService ApiContainerService) ExecuteModule(ctx context.Context, args *kurtosis_core_rpc_api_bindings.ExecuteModuleArgs) (*kurtosis_core_rpc_api_bindings.ExecuteModuleResponse, error) {
	moduleId := module.ModuleID(args.ModuleId)
	serializedParams := args.SerializedParams

	if err := apicService.metricsClient.TrackExecuteModule(args.ModuleId, serializedParams); err != nil {
		//We don't want to interrupt users flow if something fails when tracking metrics
		logrus.Errorf("An error occurred tracking execute module event\n%v", err)
	}

	serializedResult, err := apicService.moduleStore.ExecuteModule(ctx, moduleId, serializedParams)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred executing module '%v' with serialized params '%v'", moduleId, serializedParams)
	}

	resp := &kurtosis_core_rpc_api_bindings.ExecuteModuleResponse{SerializedResult: serializedResult}
	return resp, nil
}

func (apicService ApiContainerService) GetModuleInfo(ctx context.Context, args *kurtosis_core_rpc_api_bindings.GetModuleInfoArgs) (*kurtosis_core_rpc_api_bindings.GetModuleInfoResponse, error) {
	moduleId := module.ModuleID(args.ModuleId)
	privateIpAddr, privateModulePort, publicIpAddr, publicModulePort, err := apicService.moduleStore.GetModuleInfo(moduleId)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting the IP address for module '%v'", moduleId)
	}
	privateApiPort, err := transformPortSpecToApiPort(privateModulePort)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred transforming the module's private port spec port to an API port")
	}
	publicApiPort, err := transformPortSpecToApiPort(publicModulePort)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred transforming the module's public port spec port to an API port")
	}
	response := binding_constructors.NewGetModuleInfoResponse(
		privateIpAddr.String(),
		privateApiPort,
		publicIpAddr.String(),
		publicApiPort,
	)
	return response, nil
}

func (apicService ApiContainerService) RegisterFilesArtifacts(ctx context.Context, args *kurtosis_core_rpc_api_bindings.RegisterFilesArtifactsArgs) (*emptypb.Empty, error) {
	filesArtifactCache, err := apicService.enclaveDataDir.GetFilesArtifactCache()
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting the files artifact cache")
	}

	// TODO PERF: Do these in parallel
	logrus.Debug("Downloading files artifacts to the files artifact cache...")
	for artifactId, url := range args.FilesArtifactUrls {
		if err := filesArtifactCache.DownloadFilesArtifact(artifactId, url); err != nil {
			return nil, stacktrace.Propagate(err, "An error occurred downloading files artifact '%v' from URL '%v'", artifactId, url)
		}
	}
	logrus.Debug("Files artifacts downloaded successfully")

	return &emptypb.Empty{}, nil
}

func (apicService ApiContainerService) RegisterService(ctx context.Context, args *kurtosis_core_rpc_api_bindings.RegisterServiceArgs) (*kurtosis_core_rpc_api_bindings.RegisterServiceResponse, error) {
	serviceId := kurtosis_backend_service.ServiceID(args.ServiceId)
	partitionId := service_network_types.PartitionID(args.PartitionId)

	privateIpAddr, relativeServiceDirpath, err := apicService.serviceNetwork.RegisterService(serviceId, partitionId)
	if err != nil {
		// TODO IP: Leaks internal information about API container
		return nil, stacktrace.Propagate(err, "An error occurred registering apicService '%v' in the apicService network", serviceId)
	}

	return &kurtosis_core_rpc_api_bindings.RegisterServiceResponse{
		PrivateIpAddr:          privateIpAddr.String(),
		RelativeServiceDirpath: relativeServiceDirpath,
	}, nil
}

func (apicService ApiContainerService) StartService(ctx context.Context, args *kurtosis_core_rpc_api_bindings.StartServiceArgs) (*kurtosis_core_rpc_api_bindings.StartServiceResponse, error) {
	logrus.Debugf("Received request to start apicService with the following args: %+v", args)
	serviceId := kurtosis_backend_service.ServiceID(args.ServiceId)
	privateApiPorts := args.PrivatePorts
	privateServicePortSpecs := map[string]*port_spec.PortSpec{}
	for portId, privateApiPort := range privateApiPorts {
		privateServicePortSpec, err := transformApiPortToPortSpec(privateApiPort)
		if err != nil {
			return nil, stacktrace.NewError("An error occurred transforming the API port for private port '%v' into a port spec port", portId)
		}
		privateServicePortSpecs[portId] = privateServicePortSpec
	}
	// TODO REMOVE
	oldFilesArtifactMountDirPathsKeyedByFilesArtifactId := map[kurtosis_backend_service.FilesArtifactID]string{}
	for filesArtifactIdStr, mountDirPath := range args.FilesArtifactMountDirpaths {
		oldFilesArtifactMountDirPathsKeyedByFilesArtifactId[kurtosis_backend_service.FilesArtifactID(filesArtifactIdStr)] = mountDirPath
	}
	filesArtifactMountpointsByArtifactId := map[kurtosis_backend_service.FilesArtifactID]string{}
	for filesArtifactIdStr, mountDirPath := range args.FilesArtifactMountpoints {
		filesArtifactMountpointsByArtifactId[kurtosis_backend_service.FilesArtifactID(filesArtifactIdStr)] = mountDirPath
	}
	maybePublicIpAddr, publicServicePortSpecs, err := apicService.serviceNetwork.StartService(
		ctx,
		serviceId,
		args.DockerImage,
		privateServicePortSpecs,
		args.EntrypointArgs,
		args.CmdArgs,
		args.DockerEnvVars,
		args.EnclaveDataDirMntDirpath,
		oldFilesArtifactMountDirPathsKeyedByFilesArtifactId,
		filesArtifactMountpointsByArtifactId,
	)
	if err != nil {
		// TODO IP: Leaks internal information about the API container
		return nil, stacktrace.Propagate(err, "An error occurred starting the apicService in the apicService network")
	}
	publicApiPorts, err := transformPortSpecMapToApiPortsMap(publicServicePortSpecs)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred transforming the apicService's public port specs to API ports")
	}
	publicIpAddrStr := missingPublicIpAddrStr
	if maybePublicIpAddr != nil {
		publicIpAddrStr = maybePublicIpAddr.String()
	}
	response := binding_constructors.NewStartServiceResponse(publicIpAddrStr, publicApiPorts)

	serviceStartLoglineSuffix := ""
	if len(publicServicePortSpecs) > 0 {
		serviceStartLoglineSuffix = fmt.Sprintf(
			" with the following public ports: %+v",
			publicServicePortSpecs,
		)
	}
	logrus.Infof("Started apicService '%v'%v", serviceId, serviceStartLoglineSuffix)

	return response, nil
}

func (apicService ApiContainerService) GetServiceInfo(ctx context.Context, args *kurtosis_core_rpc_api_bindings.GetServiceInfoArgs) (*kurtosis_core_rpc_api_bindings.GetServiceInfoResponse, error) {
	serviceIdStr := args.GetServiceId()
	serviceId := kurtosis_backend_service.ServiceID(serviceIdStr)
	privateIpAddr, relativeServiceDirpath, err := apicService.serviceNetwork.GetServiceRegistrationInfo(serviceId)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting the registration info for apicService '%v'", serviceIdStr)
	}

	privateServicePortSpecs, maybePublicIpAddr, maybePublicServicePortSpecs, enclaveDataDirMntDirpath, err := apicService.serviceNetwork.GetServiceRunInfo(serviceId)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting the run info for apicService '%v'", serviceIdStr)
	}
	privateApiPorts, err := transformPortSpecMapToApiPortsMap(privateServicePortSpecs)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred transforming the apicService's private port spec ports to API ports")
	}
	publicIpAddrStr := missingPublicIpAddrStr
	if maybePublicIpAddr != nil {
		publicIpAddrStr = maybePublicIpAddr.String()
	}
	publicApiPorts := map[string]*kurtosis_core_rpc_api_bindings.Port{}
	if maybePublicServicePortSpecs != nil {
		publicApiPorts, err = transformPortSpecMapToApiPortsMap(maybePublicServicePortSpecs)
		if err != nil {
			return nil, stacktrace.Propagate(err, "An error occurred transforming the apicService's public port spec ports to API ports")
		}
	}

	serviceInfoResponse := binding_constructors.NewGetServiceInfoResponse(
		privateIpAddr.String(),
		privateApiPorts,
		publicIpAddrStr,
		publicApiPorts,
		enclaveDataDirMntDirpath,
		relativeServiceDirpath,
	)
	return serviceInfoResponse, nil
}

func (apicService ApiContainerService) RemoveService(ctx context.Context, args *kurtosis_core_rpc_api_bindings.RemoveServiceArgs) (*emptypb.Empty, error) {
	serviceId := kurtosis_backend_service.ServiceID(args.ServiceId)

	containerStopTimeoutSeconds := args.ContainerStopTimeoutSeconds
	containerStopTimeout := time.Duration(containerStopTimeoutSeconds) * time.Second

	if err := apicService.serviceNetwork.RemoveService(ctx, serviceId, containerStopTimeout); err != nil {
		// TODO IP: Leaks internal information about the API container
		return nil, stacktrace.Propagate(err, "An error occurred removing apicService with ID '%v'", serviceId)
	}
	return &emptypb.Empty{}, nil
}

func (apicService ApiContainerService) Repartition(ctx context.Context, args *kurtosis_core_rpc_api_bindings.RepartitionArgs) (*emptypb.Empty, error) {
	// No need to check for dupes here - that happens at the lowest-level call to ServiceNetwork.Repartition (as it should)
	partitionServices := map[service_network_types.PartitionID]map[kurtosis_backend_service.ServiceID]bool{}
	for partitionIdStr, servicesInPartition := range args.PartitionServices {
		partitionId := service_network_types.PartitionID(partitionIdStr)
		serviceIdSet := map[kurtosis_backend_service.ServiceID]bool{}
		for serviceIdStr := range servicesInPartition.ServiceIdSet {
			serviceId := kurtosis_backend_service.ServiceID(serviceIdStr)
			serviceIdSet[serviceId] = true
		}
		partitionServices[partitionId] = serviceIdSet
	}

	partitionConnections := map[service_network_types.PartitionConnectionID]partition_topology.PartitionConnection{}
	for partitionAStr, partitionBToConnection := range args.PartitionConnections {
		partitionAId := service_network_types.PartitionID(partitionAStr)
		for partitionBStr, connectionInfo := range partitionBToConnection.ConnectionInfo {
			partitionBId := service_network_types.PartitionID(partitionBStr)
			partitionConnectionId := *service_network_types.NewPartitionConnectionID(partitionAId, partitionBId)
			if _, found := partitionConnections[partitionConnectionId]; found {
				return nil, stacktrace.NewError(
					"Partition connection '%v' <-> '%v' was defined twice (possibly in reverse order)",
					partitionAId,
					partitionBId)
			}
			partitionConnection := partition_topology.PartitionConnection{
				PacketLossPercentage: connectionInfo.PacketLossPercentage,
			}
			partitionConnections[partitionConnectionId] = partitionConnection
		}
	}

	defaultConnectionInfo := args.DefaultConnection
	defaultConnection := partition_topology.PartitionConnection{
		PacketLossPercentage: defaultConnectionInfo.PacketLossPercentage,
	}

	if err := apicService.serviceNetwork.Repartition(
		ctx,
		partitionServices,
		partitionConnections,
		defaultConnection); err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred repartitioning the test network")
	}
	return &emptypb.Empty{}, nil
}

func (apicService ApiContainerService) ExecCommand(ctx context.Context, args *kurtosis_core_rpc_api_bindings.ExecCommandArgs) (*kurtosis_core_rpc_api_bindings.ExecCommandResponse, error) {
	serviceIdStr := args.ServiceId
	serviceId := kurtosis_backend_service.ServiceID(serviceIdStr)
	command := args.CommandArgs
	exitCode, logOutput, err := apicService.serviceNetwork.ExecCommand(ctx, serviceId, command)
	if err != nil {
		return nil, stacktrace.Propagate(
			err,
			"An error occurred running exec command '%v' against apicService '%v' in the apicService network",
			command,
			serviceId)
	}
	numLogOutputBytes := len(logOutput)
	if numLogOutputBytes > maxLogOutputSizeBytes {
		return nil, stacktrace.NewError(
			"Log output from docker exec command '%+v' was %v bytes, but maximum size allowed by Kurtosis is %v",
			command,
			numLogOutputBytes,
			maxLogOutputSizeBytes,
		)
	}
	resp := &kurtosis_core_rpc_api_bindings.ExecCommandResponse{
		ExitCode:  exitCode,
		LogOutput: logOutput,
	}
	return resp, nil
}

func (apicService ApiContainerService) WaitForHttpGetEndpointAvailability(ctx context.Context, args *kurtosis_core_rpc_api_bindings.WaitForHttpGetEndpointAvailabilityArgs) (*emptypb.Empty, error) {

	serviceIdStr := args.ServiceId

	if err := apicService.waitForEndpointAvailability(
		serviceIdStr,
		http.MethodGet,
		args.Port,
		args.Path,
		args.InitialDelayMilliseconds,
		args.Retries,
		args.RetriesDelayMilliseconds,
		"",
		args.BodyText); err != nil {
		return nil, stacktrace.Propagate(
			err,
			"An error occurred waiting for HTTP endpoint '%v' to become available",
			args.Path,
		)
	}

	return &emptypb.Empty{}, nil
}

func (apicService ApiContainerService) WaitForHttpPostEndpointAvailability(ctx context.Context, args *kurtosis_core_rpc_api_bindings.WaitForHttpPostEndpointAvailabilityArgs) (*emptypb.Empty, error) {

	serviceIdStr := args.ServiceId

	if err := apicService.waitForEndpointAvailability(
		serviceIdStr,
		http.MethodPost,
		args.Port,
		args.Path,
		args.InitialDelayMilliseconds,
		args.Retries,
		args.RetriesDelayMilliseconds,
		args.RequestBody,
		args.BodyText); err != nil {
		return nil, stacktrace.Propagate(
			err,
			"An error occurred waiting for HTTP endpoint '%v' to become available",
			args.Path,
		)
	}

	return &emptypb.Empty{}, nil
}

func (apicService ApiContainerService) GetServices(ctx context.Context, empty *emptypb.Empty) (*kurtosis_core_rpc_api_bindings.GetServicesResponse, error) {

	serviceIDs := make(map[string]bool, len(apicService.serviceNetwork.GetServiceIDs()))

	for serviceID := range apicService.serviceNetwork.GetServiceIDs() {
		serviceIDStr := string(serviceID)
		if _, ok := serviceIDs[serviceIDStr]; !ok {
			serviceIDs[serviceIDStr] = true
		}
	}

	resp := &kurtosis_core_rpc_api_bindings.GetServicesResponse{
		ServiceIds: serviceIDs,
	}
	return resp, nil
}

func (apicService ApiContainerService) GetModules(ctx context.Context, empty *emptypb.Empty) (*kurtosis_core_rpc_api_bindings.GetModulesResponse, error) {

	allModuleIDs := make(map[string]bool, len(apicService.moduleStore.GetModules()))

	for moduleID := range apicService.moduleStore.GetModules() {
		moduleIDStr := string(moduleID)
		if _, ok := allModuleIDs[moduleIDStr]; !ok {
			allModuleIDs[moduleIDStr] = true
		}
	}

	resp := &kurtosis_core_rpc_api_bindings.GetModulesResponse{
		ModuleIds: allModuleIDs,
	}
	return resp, nil
}

func (apicService ApiContainerService) UploadFilesArtifact(ctx context.Context, args *kurtosis_core_rpc_api_bindings.UploadFilesArtifactArgs) (*kurtosis_core_rpc_api_bindings.UploadFilesArtifactResponse, error) {
	store, err := apicService.enclaveDataDir.GetFilesArtifactStore()
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred with files artifact storage initialization.")
	}
	reader := bytes.NewReader(args.Data)

	uuid, err := store.StoreFile(reader)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred while trying to store files.")
	}

	response := &kurtosis_core_rpc_api_bindings.UploadFilesArtifactResponse{Uuid: uuid}
	return response, nil
}


func (apicService ApiContainerService) StoreWebFilesArtifact(ctx context.Context, args *kurtosis_core_rpc_api_bindings.StoreWebFilesArtifactArgs) (*kurtosis_core_rpc_api_bindings.StoreWebFilesArtifactResponse, error) {
	url := args.Url

	store, err := apicService.enclaveDataDir.GetFilesArtifactStore()
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting the files artifact store")
	}

	resp, err := http.Get(args.Url)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred making the request to URL '%v' to get the files artifact bytes", url)
	}
	defer resp.Body.Close()
	body := bufio.NewReader(resp.Body)

	uuid, err := store.StoreFile(body)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred storing the file from URL '%v' in the files artifact store", url)
	}

	response := &kurtosis_core_rpc_api_bindings.StoreWebFilesArtifactResponse{Uuid: uuid}
	return response, nil
}

func (apicService ApiContainerService) StoreFilesArtifactFromService(ctx context.Context, args *kurtosis_core_rpc_api_bindings.StoreFilesArtifactFromServiceArgs) (*kurtosis_core_rpc_api_bindings.StoreFilesArtifactFromServiceResponse, error) {
	serviceIdStr := args.ServiceId
	serviceId := kurtosis_backend_service.ServiceID(serviceIdStr)
	srcPath := args.FilesArtifactPath

	readCloser, err := apicService.serviceNetwork.CopyFromService(ctx, serviceId, srcPath)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred copying source '%v' from service with ID '%v'", srcPath, serviceId)
	}
	defer readCloser.Close()

	//Then creates a new tgz file in a temporary directory
	tarGzipFileFilepath, err := gzipCompressFile(readCloser)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred creating new temporary tar-gzip-file")
	}

	//Then opens the .tgz file
	tarGzipFile, err := os.Open(tarGzipFileFilepath)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred opening file '%v'", tarGzipFileFilepath)
	}
	defer tarGzipFile.Close()

	store, err := apicService.enclaveDataDir.GetFilesArtifactStore()
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting the files artifact store")
	}

	//And finally pass it the .tgz file to the artifact file store
	uuid, err := store.StoreFile(tarGzipFile)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred while trying to store file '%v' into the artifact file store", tarGzipFileFilepath)
	}

	response := &kurtosis_core_rpc_api_bindings.StoreFilesArtifactFromServiceResponse{Uuid: uuid}
	return response, nil
}

// ====================================================================================================
// 									   Private helper methods
// ====================================================================================================
func transformApiPortToPortSpec(port *kurtosis_core_rpc_api_bindings.Port) (*port_spec.PortSpec, error) {
	portNumUint32 := port.GetNumber()
	apiProto := port.GetProtocol()
	if portNumUint32 > math.MaxUint16 {
		return nil, stacktrace.NewError(
			"API port num '%v' is bigger than max allowed port spec port num '%v'",
			portNumUint32,
			math.MaxUint16,
		)
	}
	portNumUint16 := uint16(portNumUint32)
	portSpecProto, found := apiContainerPortProtoToPortSpecPortProto[apiProto]
	if !found {
		return nil, stacktrace.NewError("Couldn't find a port spec proto for API port proto '%v'; this should never happen, and is a bug in Kurtosis!", apiProto.String())
	}
	result, err := port_spec.NewPortSpec(portNumUint16, portSpecProto)
	if err != nil {
		return nil, stacktrace.Propagate(
			err,
			"An error occurred creating port spec object with port num '%v' and protocol '%v'",
			portNumUint16,
			portSpecProto,
		)
	}
	return result, nil
}

func transformPortSpecToApiPort(port *port_spec.PortSpec) (*kurtosis_core_rpc_api_bindings.Port, error) {
	portNumUint16 := port.GetNumber()
	portSpecProto := port.GetProtocol()
	// Yes, this isn't the most efficient way to do this, but the map is tiny so it doesn't matter
	var apiProto kurtosis_core_rpc_api_bindings.Port_Protocol
	foundApiProto := false
	for mappedApiProto, mappedPortSpecProto := range apiContainerPortProtoToPortSpecPortProto {
		if portSpecProto == mappedPortSpecProto {
			apiProto = mappedApiProto
			foundApiProto = true
			break
		}
	}
	if !foundApiProto {
		return nil, stacktrace.NewError("Couldn't find an API port proto for port spec port proto '%v'; this should never happen, and is a bug in Kurtosis!", portSpecProto)
	}
	result := binding_constructors.NewPort(uint32(portNumUint16), apiProto)
	return result, nil
}

func transformPortSpecMapToApiPortsMap(apiPorts map[string]*port_spec.PortSpec) (map[string]*kurtosis_core_rpc_api_bindings.Port, error) {
	result := map[string]*kurtosis_core_rpc_api_bindings.Port{}
	for portId, portSpec := range apiPorts {
		publicApiPort, err := transformPortSpecToApiPort(portSpec)
		if err != nil {
			return nil, stacktrace.NewError("An error occurred transforming port spec for port '%v' into an API port", portId)
		}
		result[portId] = publicApiPort
	}
	return result, nil
}

func (apicService ApiContainerService) waitForEndpointAvailability(
	serviceIdStr string,
	httpMethod string,
	port uint32,
	path string,
	initialDelayMilliseconds uint32,
	retries uint32,
	retriesDelayMilliseconds uint32,
	requestBody string,
	bodyText string) error {

	var (
		resp *http.Response
		err  error
	)

	privateServiceIp, _, err := apicService.serviceNetwork.GetServiceRegistrationInfo(kurtosis_backend_service.ServiceID(serviceIdStr))
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred getting the registration info for apicService '%v'", serviceIdStr)
	}

	url := fmt.Sprintf("http://%v:%v/%v", privateServiceIp, port, path)

	time.Sleep(time.Duration(initialDelayMilliseconds) * time.Millisecond)

	for i := uint32(0); i < retries; i++ {
		resp, err = makeHttpRequest(httpMethod, url, requestBody)
		if err == nil {
			break
		}
		time.Sleep(time.Duration(retriesDelayMilliseconds) * time.Millisecond)
	}

	if err != nil {
		return stacktrace.Propagate(
			err,
			"The HTTP endpoint '%v' didn't return a success code, even after %v retries with %v milliseconds in between retries",
			url,
			retries,
			retriesDelayMilliseconds,
		)
	}

	if bodyText != "" {
		body := resp.Body
		defer body.Close()

		bodyBytes, err := ioutil.ReadAll(body)

		if err != nil {
			return stacktrace.Propagate(err,
				"An error occurred reading the response body from endpoint '%v'", url)
		}

		bodyStr := string(bodyBytes)

		if bodyStr != bodyText {
			return stacktrace.NewError("Expected response body text '%v' from endpoint '%v' but got '%v' instead", bodyText, url, bodyStr)
		}
	}

	return nil
}

func makeHttpRequest(httpMethod string, url string, body string) (*http.Response, error) {
	var (
		resp *http.Response
		err  error
	)

	if httpMethod == http.MethodPost {
		var bodyByte = []byte(body)
		resp, err = http.Post(url, "application/json", bytes.NewBuffer(bodyByte))
	} else if httpMethod == http.MethodGet {
		resp, err = http.Get(url)
	} else {
		return nil, stacktrace.NewError("HTTP method '%v' not allowed", httpMethod)
	}

	if err != nil {
		return nil, stacktrace.Propagate(err, "An HTTP error occurred when sending GET request to endpoint '%v'", url)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, stacktrace.NewError("Received non-OK status code: '%v'", resp.StatusCode)
	}
	return resp, nil
}

func gzipCompressFile(readCloser io.Reader) (resultFilepath string, resultErr error) {
	useDefaultDirectoryArg := ""
	withoutPatternArg := ""
	tgzFile, err := ioutil.TempFile(useDefaultDirectoryArg,withoutPatternArg)
	if err != nil {
		return "", stacktrace.Propagate(err,
			"There was an error creating a temporary file")
	}
	defer tgzFile.Close()
	gzipCompressingWriter := gzip.NewWriter(tgzFile)

	tarGzipFileFilepath := tgzFile.Name()
	if _, err := io.Copy(gzipCompressingWriter, readCloser); err != nil {
		return "", stacktrace.Propagate(err, "An error occurred copying content from tar reader to file '%v'", tarGzipFileFilepath)
	}

	return tarGzipFileFilepath, nil
}

func createTemporaryTarGzFileFromTarReader(tarReader *tar.Reader) (string, error) {

	tarContentTempDir, err := saveTarContentInTempDir(tarReader)

	tarGzipFileFilepath, err := archiveFiles(tarContentTempDir)
	if err != nil {
		return "", stacktrace.Propagate(err, "An error occurred creating a new gzip-tar-file for the content saved in temporary directory '%v'", tarContentTempDir)
	}

	return tarGzipFileFilepath, nil
}

func saveTarContentInTempDir(tarReader *tar.Reader) (string, error) {
	useDefaultDirectoryArg := ""
	withoutPatternArg := ""
	tempDirectoryName, err := ioutil.TempDir(useDefaultDirectoryArg,withoutPatternArg)
	if err != nil {
		return "", stacktrace.Propagate(err, "An error occurred creating a new temporary directory to save tar file content")
	}

	for {
		tarHeader, err := tarReader.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", stacktrace.Propagate(err, "And error occurred reading reading tar file")
		}

		destinationFilepath := path.Join(tempDirectoryName, tarHeader.Name)

		switch tarHeader.Typeflag {
		case tar.TypeDir:
			if err := createNewDirInTempDir(destinationFilepath); err != nil {
				return "", stacktrace.Propagate(err, "An error occurred creating new directory '%v'", destinationFilepath)
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := createNewFileInTempDir(destinationFilepath, tarReader); err != nil {
				return "", stacktrace.Propagate(err, "An error occurred creating new file '%v'", destinationFilepath)
			}
		default:
			return "", stacktrace.Propagate(err, "File type '%v' of file '%v it is not supported for copying tar content between nodes", tarHeader.Typeflag, tarHeader.Name)
		}

		// maintaining access and modification time in best effort fashion
		if err := os.Chtimes(destinationFilepath, tarHeader.AccessTime, tarHeader.ModTime); err != nil {
			//We don't want to block the entire process because we were not able to update time files
			logrus.Debugf("An error occurred updating access and modifications times for '%v' using acces time '%v' and modification time '%v' from tar file '%v'", destinationFilepath, tarHeader.AccessTime, tarHeader.ModTime, tarHeader.Name)
		}
	}

	return tempDirectoryName, nil
}

func createNewDirInTempDir(dirpath string) error {
	if err := os.Mkdir(dirpath, TempDirFileFileMode); err != nil {
		if !os.IsExist(err) {
			return stacktrace.Propagate(err, "An error occurred creating new directory '%v' with file mode '%v'", dirpath, TempDirFileFileMode)
		}
		//If dir exist change update permissions
		err = os.Chmod(dirpath, TempDirFileFileMode)
		if err != nil {
			return stacktrace.Propagate(err, "An error occurred changing directory permissions for '%v' with file mode '%v'", dirpath, TempDirFileFileMode)
		}
	}
	return nil
}

func createNewFileInTempDir(filepath string, tarReader *tar.Reader) error {
	openFileFlags := os.O_CREATE|os.O_TRUNC|os.O_WRONLY
	file, err := os.OpenFile(filepath, openFileFlags, TempDirFileFileMode)
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred opening file '%v' using flags '%v' and file mode '%v'", filepath, openFileFlags, TempDirFileFileMode)
	}
	defer file.Close()
	if _, err := io.Copy(file, tarReader); err != nil {
		return stacktrace.Propagate(err, "An error occurred copying content from tar reader to file '%v' in filepath '%v'", file.Name(), filepath)
	}
	return nil
}

func archiveFiles(pathToUpload string) (string, error) {
	useDefaultDirectoryArg := ""
	withoutPatternArg := ""
	tarFile, err := ioutil.TempFile(useDefaultDirectoryArg,withoutPatternArg)
	if err != nil {
		return "", stacktrace.Propagate(err,
			"There was an error creating a temporary archive file at '%s' during files upload for '%s'.",
			tarFile.Name(), pathToUpload)
	}
	defer tarFile.Close()
	gzipWriter := gzip.NewWriter(tarFile)
	defer gzipWriter.Close()
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	if err = filepath.Walk(pathToUpload, func(filePath string, fileInfo os.FileInfo, err error) error {
		return addFilesToArchive(filePath, fileInfo, err, tarWriter, pathToUpload)
	}); err != nil {
		return "", stacktrace.Propagate(err,
			"There was an error searching through directory '%s' during the archive process.", pathToUpload)
	}

	return tarFile.Name(), nil
}

func addFilesToArchive(filePath string, fileInfo os.FileInfo, errorFromLastIteration error, archiveWriter *tar.Writer, pathToArchive string) error {
	if errorFromLastIteration != nil {
		return stacktrace.Propagate(errorFromLastIteration,
			"There was an error while taring or accessing file at '%s'.", filePath)
	}

	if !fileInfo.Mode().IsRegular() {
		return nil
	}

	pathToArchiveWithLastForwardSlash := pathToArchive + "/"

	//Create a file header that determines the relative path, w.r.t extraction, where the file belongs.
	header := &tar.Header{
		Name:    strings.Replace(filePath, pathToArchiveWithLastForwardSlash, "", 1),
		Size:    fileInfo.Size(),
		Mode:    int64(fileInfo.Mode()),
		ModTime: fileInfo.ModTime(),
	}

	if err := archiveWriter.WriteHeader(header); err != nil {
		return stacktrace.Propagate(err, "There was a problem writing headers while archiving '%s'.", filePath)
	}

	sourceToArchive, err := os.Open(filePath)
	if err != nil {
		return stacktrace.Propagate(err, "There was a problem reading from '%s'.", filePath)
	}
	defer sourceToArchive.Close()
	if _, err := io.Copy(archiveWriter, sourceToArchive); err != nil {
		return stacktrace.Propagate(err, "There was a problem copying '%s' to the tar.", filePath)
	}
	return nil
}