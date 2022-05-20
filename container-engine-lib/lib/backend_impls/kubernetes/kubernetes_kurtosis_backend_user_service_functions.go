package kubernetes
import (
	"bytes"
	"context"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_impls/kubernetes/kubernetes_resource_collectors"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_impls/kubernetes/object_attributes_provider"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_impls/kubernetes/object_attributes_provider/annotation_key_consts"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_impls/kubernetes/object_attributes_provider/kubernetes_label_key"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_impls/kubernetes/object_attributes_provider/kubernetes_label_value"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_impls/kubernetes/object_attributes_provider/kubernetes_port_spec_serializer"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_impls/kubernetes/object_attributes_provider/label_key_consts"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_impls/kubernetes/object_attributes_provider/label_value_consts"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/container_status"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/enclave"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/exec_result"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/files_artifact_expansion_volume"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/port_spec"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/service"
	"github.com/kurtosis-tech/container-engine-lib/lib/uuid_generator"
	"github.com/kurtosis-tech/stacktrace"
	"github.com/sirupsen/logrus"
	"io"
	apiv1 "k8s.io/api/core/v1"
	"net"
)

/*
      *************************************************************************************************
      * See the documentation on the KurtosisBackend interface for the Kurtosis service state diagram *
      *************************************************************************************************

                                  KUBERNETES SERVICES IMPLEMENTATION

States:
- REGISTERED: a ServiceGUID has been generated, a Kubernetes Service has been created with that ServiceGUID as a label,
	the Kubernetes Service gets it selector set to look for pods with that ServiceGUID (of which there will be none at
	first), and the ClusterIP of the Service is returned to the caller.
- RUNNING: one or more Pods match the ServiceGUID selector of the Service.
- STOPPED = the Service's selectors are set to empty (this is to distinguish between "no Pods exist because they haven't
	been started yet", i.e. REGISTERED, and "no Pods exist because they were stopped", i.e. STOPPED).
- DESTROYED = no Service or Pod for the ServiceGUID exists.

State transitions:
- RegisterService: a Service is created with the ServiceGUID label (for finding later), the Service's selectors are set
	to look for Pods with the ServiceGUID, and the Service's ClusterIP is returned to the caller.
- StartService: a Pod is created with the ServiceGUID, the Kubernetes Service gets its ports updated with the requested
	ports, and the ServiceGUID selector will mean that the Service will automatically connect to the Pod.
- StopServices: The Pod is destroyed (because Kubernetes doesn't have a way to stop Pods - only remove them) and the Service's
	selectors are set to empty. If we want to keep Kubernetes logs around after a Pod is destroyed, we'll need to implement
	our own logstore.
- DestroyServices: the Service is destroyed, as is any Pod that may have been started.

Implementation notes:
- Kubernetes has no concept of stopping a pod without removing it; if we want Pod logs we'll need to implement our own logstore.
- The IP that the container gets as its "own IP" is technically the IP of the Service. This *should* be fine, but we'll need
	to keep an eye out for it.
*/

const (
	userServiceContainerName = "user-service-container"

	shouldMountVolumesAsReadOnly = false
	shouldAddTimestampsToLogs = false
	// Our user services don't need service accounts
	userServiceServiceAccountName = ""

	// Kubernetes doesn't allow us to create services without ports exposed, but we might not have ports in the following situations:
	//  1) we've registered a service but haven't started a container yet (so ports are yet to come)
	//  2) we've started a container that doesn't listen on any ports
	// In these cases, we use these notional unbound ports
	unboundPortName = "nonexistent-port"
	unboundPortNumber = 1
)

// Kubernetes doesn't provide public IP or port information; this is instead handled by the Kurtosis gateway that the user uses
// to connect to Kubernetes
var servicePublicIp net.IP = nil
var servicePublicPorts map[string]*port_spec.PortSpec = nil

type userServiceObjectsAndKubernetesResources struct {
	// Should never be nil because 1 Kubernetes service = 1 Kurtosis service registration
	serviceRegistration *service.ServiceRegistration

	// May be nil if no pod has been started yet
	service *service.Service

	// Will never be nil
	kubernetesResources *userServiceKubernetesResources
}

// Any of these fields can be nil if they don't exist in Kubernetes, though at least
// one field will be present (else this struct won't exist)
type userServiceKubernetesResources struct {
	// This can be nil if the user manually deleted the Kubernetes service (e.g. using the Kubernetes dashboard)
	service *apiv1.Service

	// This can be nil if the user hasn't started a pod for the service yet, or if the pod was deleted
	pod *apiv1.Pod
}

func (backend *KubernetesKurtosisBackend) RegisterUserService(ctx context.Context, enclaveId enclave.EnclaveID, serviceId service.ServiceID) (*service.ServiceRegistration, error) {
	namespaceName, err := backend.getEnclaveNamespaceName(ctx, enclaveId)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting namespace name for enclave '%v'", enclaveId)
	}

	serviceGuidStr, err := uuid_generator.GenerateUUIDString()
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred generating a UUID to use for the service GUID")
	}
	serviceGuid := service.ServiceGUID(serviceGuidStr)

	objectAttributesProvider := object_attributes_provider.GetKubernetesObjectAttributesProvider()
	enclaveObjAttributesProvider := objectAttributesProvider.ForEnclave(enclaveId)

	serviceAttributes, err := enclaveObjAttributesProvider.ForUserServiceService(serviceGuid, serviceId)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting attributes for the Kubernetes service for user service '%v'", serviceId)
	}

	serviceNameStr := serviceAttributes.GetName().GetString()

	serviceLabelsStrs := getStringMapFromLabelMap(serviceAttributes.GetLabels())
	serviceAnnotationsStrs := getStringMapFromAnnotationMap(serviceAttributes.GetAnnotations())

	// Set up the labels that the pod will match (i.e. the labels of the pod-to-be)
	// WARNING: We *cannot* use the labels of the Service itself because we're not guaranteed that the labels
	//  between the two will be identical!
	serviceGuidLabelValue, err := kubernetes_label_value.CreateNewKubernetesLabelValue(string(serviceGuid))
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred creating a Kubernetes pod match label value for the service GUID '%v'", serviceGuid)
	}
	enclaveIdLabelValue, err := kubernetes_label_value.CreateNewKubernetesLabelValue(string(enclaveId))
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred creating a Kubernetes pod match label value for the enclave ID '%v'", enclaveId)
	}
	matchedPodLabels := map[*kubernetes_label_key.KubernetesLabelKey]*kubernetes_label_value.KubernetesLabelValue{
		label_key_consts.AppIDKubernetesLabelKey: label_value_consts.AppIDKubernetesLabelValue,
		label_key_consts.EnclaveIDKubernetesLabelKey: enclaveIdLabelValue,
		label_key_consts.GUIDKubernetesLabelKey: serviceGuidLabelValue,
	}
	matchedPodLabelStrs := getStringMapFromLabelMap(matchedPodLabels)

	// Kubernetes doesn't allow us to create services without any ports, so we need to set this to a notional value
	// until the user calls StartService
	notionalServicePorts := []apiv1.ServicePort{
		{
			Name:        unboundPortName,
			Port:        unboundPortNumber,
		},
	}

	createdService, err := backend.kubernetesManager.CreateService(
		ctx,
		namespaceName,
		serviceNameStr,
		serviceLabelsStrs,
		serviceAnnotationsStrs,
		matchedPodLabelStrs,
		apiv1.ServiceTypeClusterIP,
		notionalServicePorts,
	)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred creating Kubernetes service in enclave '%v' with ID '%v'", enclaveId, serviceId)
	}
	shouldDeleteService := true
	defer func() {
		if shouldDeleteService {
			if err := backend.kubernetesManager.RemoveService(ctx, namespaceName, createdService.Name); err != nil {
				logrus.Errorf("Registering service '%v' didn't complete successfully so we tried to remove the Kubernetes service we created but doing so threw an error:\n%v", serviceId, err)
				logrus.Errorf("ACTION REQUIRED: You'll need to remove service '%v' in namespace '%v' manually!!!", createdService.Name, namespaceName)
			}
		}
	}()

	kubernetesResources := map[service.ServiceGUID]*userServiceKubernetesResources{
		serviceGuid: {
			service:   createdService,
			pod: nil,	// No pod yet
		},
	}

	convertedObjects, err := getUserServiceObjectsFromKubernetesResources(enclaveId, kubernetesResources)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting a service registration object from Kubernetes service")
	}
	objectsAndResources, found := convertedObjects[serviceGuid]
	if !found {
		return nil, stacktrace.NewError(
			"Successfully converted the Kubernetes service representing registered service with GUID '%v' to a " +
				"Kurtosis object, but couldn't find that key in the resulting map; this is a bug in Kurtosis",
			serviceGuid,
		)
	}
	serviceRegistration := objectsAndResources.serviceRegistration

	shouldDeleteService = false
	return serviceRegistration, nil
}

func (backend *KubernetesKurtosisBackend) StartUserService(
	ctx context.Context,
	enclaveId enclave.EnclaveID,
	serviceGuid service.ServiceGUID,
	containerImageName string,
	privatePorts map[string]*port_spec.PortSpec,
	entrypointArgs []string,
	cmdArgs []string,
	envVars map[string]string,
	// TODO This needs a redo - at minimum it should be by files_artifact_expansion_volume_guid
	filesArtifactVolumeMountDirpaths map[files_artifact_expansion_volume.FilesArtifactExpansionVolumeName]string,
) (
	newUserService *service.Service,
	resultErr error,
) {
	preexistingServiceFilters := &service.ServiceFilters{
		GUIDs:    map[service.ServiceGUID]bool{
			serviceGuid: true,
		},
	}
	preexistingObjectsAndResources, err := backend.getMatchingUserServiceObjectsAndKubernetesResources(
		ctx,
		enclaveId,
		preexistingServiceFilters,
	)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting user service objects and Kubernetes resources matching service GUID '%v'", serviceGuid)
	}
	if len(preexistingObjectsAndResources) == 0 {
		return nil, stacktrace.NewError("Couldn't find any service registrations matching service GUID '%v'", serviceGuid)
	}
	if len(preexistingObjectsAndResources) > 1 {
		// Should never happen because service GUIDs should be unique
		return nil, stacktrace.NewError("Found more than one service registration matching service GUID '%v'; this is a bug in Kurtosis", serviceGuid)
	}
	matchingObjectAndResources, found := preexistingObjectsAndResources[serviceGuid]
	if !found {
		return nil, stacktrace.NewError("Even though we pulled back some Kubernetes resources, no Kubernetes resources were available for requested service GUID '%v'; this is a bug in Kurtosis", serviceGuid)
	}
	kubernetesService := matchingObjectAndResources.kubernetesResources.service
	serviceObj := matchingObjectAndResources.service
	if serviceObj != nil {
		return nil, stacktrace.NewError("Cannot start service with GUID '%v' because the service has already been started previously", serviceGuid)
	}

	namespaceName := kubernetesService.GetNamespace()
	serviceRegistrationObj := matchingObjectAndResources.serviceRegistration

	// Create the pod
	objectAttributesProvider := object_attributes_provider.GetKubernetesObjectAttributesProvider()
	enclaveObjAttributesProvider := objectAttributesProvider.ForEnclave(enclaveId)
	podAttributes, err := enclaveObjAttributesProvider.ForUserServicePod(serviceGuid, serviceRegistrationObj.GetID(), privatePorts)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting attributes for new pod for service with GUID '%v'", serviceGuid)
	}

	podLabelsStrs := getStringMapFromLabelMap(podAttributes.GetLabels())
	podAnnotationsStrs := getStringMapFromAnnotationMap(podAttributes.GetAnnotations())

	// TODO TODO THIS WON'T WORK AND IS BROKEN FOR NOW
	temporaryHackyFilesArtifactMountDirpaths := map[files_artifact_expansion_volume.FilesArtifactExpansionGUID]string{}

	podVolumes, containerMounts, err := backend.getUserServiceVolumeInfoFromFilesArtifactMountpoints(
		ctx,
		namespaceName,
		enclaveId,
		serviceGuid,
		temporaryHackyFilesArtifactMountDirpaths,
	)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting pod volumes from files artifact mountpoints: %+v", filesArtifactVolumeMountDirpaths)
	}

	podContainers, err := getUserServicePodContainerSpecs(
		containerImageName,
		entrypointArgs,
		cmdArgs,
		envVars,
		privatePorts,
		containerMounts,
	)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred creating the container specs for the user service pod with image '%v'", containerImageName)
	}

	podName := podAttributes.GetName().GetString()
	createdPod, err := backend.kubernetesManager.CreatePod(
		ctx,
		namespaceName,
		podName,
		podLabelsStrs,
		podAnnotationsStrs,
		podContainers,
		podVolumes,
		userServiceServiceAccountName,
	)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred creating pod '%v' using image '%v'", podName, containerImageName)
	}
	shouldDestroyPod := true
	defer func() {
		if shouldDestroyPod {
			if err := backend.kubernetesManager.RemovePod(ctx, namespaceName, podName); err != nil {
				logrus.Errorf("Starting service didn't complete successfully so we tried to remove the pod we created but doing so threw an error:\n%v", err)
				logrus.Errorf("ACTION REQUIRED: You'll need to remove pod '%v' in '%v' manually!!!", podName, namespaceName)
			}
		}
	}()

	updatedService, err := backend.updateServiceWhenContainerStarted(ctx, namespaceName, kubernetesService, privatePorts)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred updating service '%v' to reflect its new ports: %+v", kubernetesService.GetName(), privatePorts)
	}
	shouldUndoServiceUpdate := true
	defer func() {
		if shouldUndoServiceUpdate {
			if err := backend.kubernetesManager.UpdateService(ctx, namespaceName, kubernetesService); err != nil {
				logrus.Errorf("Starting service didn't complete successfully so we tried to undo the changes we made to the Kubernetes service but doing so threw an error:\n%v", err)
				logrus.Errorf("ACTION REQUIRED: You'll need to manually correct service '%v' in namespace '%v'!!!", kubernetesService.GetName(), namespaceName)
			}
		}
	}()

	kubernetesResources := map[service.ServiceGUID]*userServiceKubernetesResources{
		serviceGuid: {
			service: updatedService,
			pod:     createdPod,
		},
	}
	convertedObjects, err := getUserServiceObjectsFromKubernetesResources(enclaveId, kubernetesResources)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting a service object from the Kubernetes service and newly-created pod")
	}
	objectsAndResources, found := convertedObjects[serviceGuid]
	if !found {
		return nil, stacktrace.NewError(
			"Successfully converted the Kubernetes service + pod representing a running service with GUID '%v' to a " +
				"Kurtosis object, but couldn't find that key in the resulting map; this is a bug in Kurtosis",
			serviceGuid,
		)
	}

	shouldDestroyPod = false
	shouldUndoServiceUpdate = false
	return objectsAndResources.service, nil
}

func (backend *KubernetesKurtosisBackend) GetUserServices(
	ctx context.Context,
	enclaveId enclave.EnclaveID,
	filters *service.ServiceFilters,
) (successfulUserServices map[service.ServiceGUID]*service.Service, resultError error) {
	allObjectsAndResources, err := backend.getMatchingUserServiceObjectsAndKubernetesResources(ctx, enclaveId, filters)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting user services in enclave '%v' matching filters: %+v", filters)
	}
	result := map[service.ServiceGUID]*service.Service{}
	for guid, serviceObjsAndResources := range allObjectsAndResources {
		serviceObj := serviceObjsAndResources.service
		if serviceObj == nil {
			// Indicates a registration-only service; skip
			continue
		}
		result[guid] = serviceObj
	}
	return result, nil
}

func (backend *KubernetesKurtosisBackend) GetUserServiceLogs(
	ctx context.Context,
	enclaveId enclave.EnclaveID,
	filters *service.ServiceFilters,
	shouldFollowLogs bool,
) (successfulUserServiceLogs map[service.ServiceGUID]io.ReadCloser, erroredUserServiceGuids map[service.ServiceGUID]error, resultError error) {
	serviceObjectsAndResources, err := backend.getMatchingUserServiceObjectsAndKubernetesResources(ctx, enclaveId, filters)
	if err != nil {
		return nil, nil, stacktrace.Propagate(err, "Expected to be able to get user services and Kubernetes resources, instead a non nil error was returned")
	}
	userServiceLogs := map[service.ServiceGUID]io.ReadCloser{}
	erredServiceLogs := map[service.ServiceGUID]error{}
	for _, serviceObjectAndResource := range serviceObjectsAndResources {
		serviceGuid := serviceObjectAndResource.service.GetRegistration().GetGUID()
		servicePod := serviceObjectAndResource.kubernetesResources.pod
		if servicePod == nil {
			erredServiceLogs[serviceGuid] = stacktrace.NewError("Expected to find a pod for Kurtosis service with GUID '%v', instead no pod was found", serviceGuid)
			continue
		}
		serviceNamespaceName := serviceObjectAndResource.kubernetesResources.service.GetNamespace()
		// Get logs
		logReadCloser, err := backend.kubernetesManager.GetContainerLogs(ctx, serviceNamespaceName, servicePod.Name, userServiceContainerName, shouldFollowLogs, shouldAddTimestampsToLogs)
		if err != nil {
			erredServiceLogs[serviceGuid] = stacktrace.Propagate(err, "Expected to be able to call Kubernetes to get logs for service with GUID '%v', instead a non-nil error was returned", serviceGuid)
			continue
		}
		userServiceLogs[serviceGuid] = logReadCloser
	}
	return userServiceLogs, erredServiceLogs, nil
}

func (backend *KubernetesKurtosisBackend) PauseService(
	ctx context.Context,
	enclaveId enclave.EnclaveID,
	serviceId service.ServiceGUID,
) error {
	return stacktrace.NewError("Cannot pause service '%v' in enclave '%v' because pausing is not supported by Kubernetes", serviceId, enclaveId)
}

func (backend *KubernetesKurtosisBackend) UnpauseService(
	ctx context.Context,
	enclaveId enclave.EnclaveID,
	serviceId service.ServiceGUID,
) error {
	return stacktrace.NewError("Cannot pause service '%v' in enclave '%v' because unpausing is not supported by Kubernetes", serviceId, enclaveId)
}

func (backend *KubernetesKurtosisBackend) RunUserServiceExecCommands(
	ctx context.Context,
	enclaveId enclave.EnclaveID,
	userServiceCommands map[service.ServiceGUID][]string,
) (
	succesfulUserServiceExecResults map[service.ServiceGUID]*exec_result.ExecResult,
	erroredUserServiceGuids map[service.ServiceGUID]error,
	resultErr error,
) {
	namespaceName, err := backend.getEnclaveNamespaceName(ctx, enclaveId)
	if err != nil {
		return nil, nil, stacktrace.Propagate(err, "An error occurred getting namespace name for enclave '%v'", enclaveId)
	}

	requestedGuids := map[service.ServiceGUID]bool{}
	for guid := range userServiceCommands {
		requestedGuids[guid] = true
	}
	matchingServicesFilters := &service.ServiceFilters{
		GUIDs: requestedGuids,
	}
	matchingObjectsAndResources, err := backend.getMatchingUserServiceObjectsAndKubernetesResources(ctx, enclaveId, matchingServicesFilters)
	if err != nil {
		return nil, nil, stacktrace.Propagate(err, "An error occurred getting user services matching the requested GUIDs: %+v", requestedGuids)
	}

	for guid, commandArgs := range userServiceCommands {
		objectsAndResources, found := matchingObjectsAndResources[guid]
		if !found {
			return nil, nil, stacktrace.NewError(
				"Requested to execute command '%+v' on service '%v', but the service does not exist",
				commandArgs,
				guid,
			)
		}
		serviceObj := objectsAndResources.service
		if serviceObj == nil {
			return nil, nil, stacktrace.NewError(
				"Cannot execute command '%+v' on service '%v' because the service is not started yet",
				commandArgs,
				guid,
			)
		}
		if serviceObj.GetStatus() != container_status.ContainerStatus_Running {
			return nil, nil, stacktrace.NewError(
				"Cannot execute command '%+v' on service '%v' because the service status is '%v'",
				commandArgs,
				guid,
				serviceObj.GetStatus().String(),
			)
		}
	}

	userServiceExecSuccess := map[service.ServiceGUID]*exec_result.ExecResult{}
	userServiceExecErr := map[service.ServiceGUID]error{}
	for serviceGuid, serviceCommand := range userServiceCommands {
		userServiceObjectAndResources, found := matchingObjectsAndResources[serviceGuid]
		if !found {
			// Should never happen because we validate that the object exists earlier
			return nil, nil, stacktrace.NewError("Validated that service '%v' has Kubernetes resources, but couldn't find them when we need to run the exec", serviceGuid)
		}
		// Don't need to validate that this is non-nil because we did so before we started executing
		userServicePod := userServiceObjectAndResources.kubernetesResources.pod
		userServicePodName := userServicePod.Name

		outputBuffer := &bytes.Buffer{}
		exitCode, err := backend.kubernetesManager.RunExecCommand(namespaceName, userServicePodName, userServiceContainerName, serviceCommand, outputBuffer)
		if err != nil {
			userServiceExecErr[serviceGuid] = stacktrace.Propagate(
				err,
				"Expected to be able to execute command '%+v' in user service container '%v' in Kubernetes pod '%v' " +
					"for Kurtosis service with guid '%v', instead a non-nil error was returned",
				serviceCommand,
				userServiceContainerName,
				userServicePodName,
				serviceGuid,
			)
			continue
		}
		userServiceExecSuccess[serviceGuid] = exec_result.NewExecResult(exitCode, outputBuffer.String())
	}
	return userServiceExecSuccess, userServiceExecErr, nil
}

func (backend *KubernetesKurtosisBackend) GetConnectionWithUserService(ctx context.Context, enclaveId enclave.EnclaveID, serviceGUID service.ServiceGUID) (resultConn net.Conn, resultErr error) {
	// TODO IMPLEMENT
	return nil, stacktrace.NewError("Getting a connection with a user service isn't yet implemented on Kubernetes")
}

func (backend *KubernetesKurtosisBackend) CopyFromUserService(ctx context.Context, enclaveId enclave.EnclaveID, serviceGuid service.ServiceGUID, srcPath string) (resultReadCloser io.ReadCloser, resultErr error) {
	//TODO implement me
	panic("implement me")
}

func (backend *KubernetesKurtosisBackend) StopUserServices(ctx context.Context, enclaveId enclave.EnclaveID, filters *service.ServiceFilters) (resultSuccessfulGuids map[service.ServiceGUID]bool, resultErroredGuids map[service.ServiceGUID]error, resultErr error) {
	namespaceName, err := backend.getEnclaveNamespaceName(ctx, enclaveId)
	if err != nil {
		return nil, nil, stacktrace.Propagate(err, "An error occurred getting namespace name for enclave '%v'", enclaveId)
	}

	allObjectsAndResources, err := backend.getMatchingUserServiceObjectsAndKubernetesResources(ctx, enclaveId, filters)
	if err != nil {
		return nil, nil, stacktrace.Propagate(err, "An error occurred getting user services in enclave '%v' matching filters: %+v", filters)
	}

	successfulGuids := map[service.ServiceGUID]bool{}
	erroredGuids := map[service.ServiceGUID]error{}
	for serviceGuid, serviceObjsAndResources := range allObjectsAndResources {
		resources := serviceObjsAndResources.kubernetesResources

		pod := resources.pod
		if pod != nil {
			if err := backend.kubernetesManager.RemovePod(ctx, namespaceName, pod.Name); err != nil {
				erroredGuids[serviceGuid] = stacktrace.Propagate(
					err,
					"An error occurred removing Kubernetes pod '%v' in namespace '%v'",
					pod.Name,
					namespaceName,
				)
				continue
			}
		}

		kubernetesService := resources.service
		updatedService := kubernetesService.DeepCopy()
		updatedService.Spec.Selector = nil
		if err := backend.kubernetesManager.UpdateService(ctx, namespaceName, updatedService); err != nil {
			erroredGuids[serviceGuid] = stacktrace.Propagate(
				err,
				"An error occurred updating service '%v' in namespace '%v' to reflect that it's no longer running",
				kubernetesService.Name,
			)
			continue
		}

		successfulGuids[serviceGuid] = true
	}
	return successfulGuids, erroredGuids, nil
}

func (backend *KubernetesKurtosisBackend) DestroyUserServices(ctx context.Context, enclaveId enclave.EnclaveID, filters *service.ServiceFilters) (resultSuccessfulGuids map[service.ServiceGUID]bool, resultErroredGuids map[service.ServiceGUID]error, resultErr error) {
	namespaceName, err := backend.getEnclaveNamespaceName(ctx, enclaveId)
	if err != nil {
		return nil, nil, stacktrace.Propagate(err, "An error occurred getting namespace name for enclave '%v'", enclaveId)
	}

	allObjectsAndResources, err := backend.getMatchingUserServiceObjectsAndKubernetesResources(ctx, enclaveId, filters)
	if err != nil {
		return nil, nil, stacktrace.Propagate(err, "An error occurred getting user services in enclave '%v' matching filters: %+v", filters)
	}

	successfulGuids := map[service.ServiceGUID]bool{}
	erroredGuids := map[service.ServiceGUID]error{}
	for serviceGuid, serviceObjsAndResources := range allObjectsAndResources {
		resources := serviceObjsAndResources.kubernetesResources

		pod := resources.pod
		if pod != nil {
			if err := backend.kubernetesManager.RemovePod(ctx, namespaceName, pod.Name); err != nil {
				erroredGuids[serviceGuid] = stacktrace.Propagate(
					err,
					"An error occurred removing Kubernetes pod '%v' in namespace '%v'",
					pod.Name,
					namespaceName,
				)
				continue
			}
		}

		kubernetesService := resources.service
		if kubernetesService != nil {
			if err := backend.kubernetesManager.RemoveService(ctx, namespaceName, kubernetesService.Name); err != nil {
				erroredGuids[serviceGuid] = stacktrace.Propagate(
					err,
					"An error occurred removing Kubernetes service '%v' in namespace '%v'",
					kubernetesService.Name,
					namespaceName,
				)
				continue
			}
		}

		successfulGuids[serviceGuid] = true
	}
	return successfulGuids, erroredGuids, nil
}


// ====================================================================================================
//                                     Private Helper Methods
// ====================================================================================================
func (backend *KubernetesKurtosisBackend) getMatchingUserServiceObjectsAndKubernetesResources(
	ctx context.Context,
	enclaveId enclave.EnclaveID,
	filters *service.ServiceFilters,
) (
	map[service.ServiceGUID]*userServiceObjectsAndKubernetesResources,
	error,
) {
	allResources, err := backend.getUserServiceKubernetesResourcesMatchingGuids(ctx, enclaveId, filters.GUIDs)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting user service Kubernetes resources matching GUIDs: %+v", filters.GUIDs)
	}

	for serviceGuid, serviceResources := range allResources {
		logrus.Tracef("Found resources for service '%v': %+v", serviceGuid, serviceResources)
	}

	allObjectsAndResources, err := getUserServiceObjectsFromKubernetesResources(enclaveId, allResources)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting user service objects from Kubernetes resources")
	}

	// Sanity check
	if len(allResources) != len(allObjectsAndResources) {
		return nil, stacktrace.NewError(
			"Transformed %v Kubernetes resource objects into %v Kurtosis objects; this is a bug in Kurtosis",
			len(allResources),
			len(allObjectsAndResources),
		)
	}

	// Filter the results down to the requested filters
	results := map[service.ServiceGUID]*userServiceObjectsAndKubernetesResources{}
	for serviceGuid, objectsAndResources := range allObjectsAndResources {
		if filters.GUIDs != nil && len(filters.GUIDs) > 0 {
			if _, found := filters.GUIDs[serviceGuid]; !found {
				continue
			}
		}

		registration := objectsAndResources.serviceRegistration
		if filters.IDs != nil && len(filters.IDs) > 0 {
			if _, found := filters.IDs[registration.GetID()]; !found {
				continue
			}
		}

		if filters.Statuses != nil && len(filters.Statuses) > 0 {
			kubernetesService := objectsAndResources.service

			// If status isn't specified, return registered-only objects; if not, remove them all
			if kubernetesService == nil {
				continue
			}

			if _, found := filters.Statuses[kubernetesService.GetStatus()]; !found {
				continue
			}
		}

		results[serviceGuid] = objectsAndResources
	}

	return results, nil
}

func (backend *KubernetesKurtosisBackend) getSingleUserServiceObjectsAndResources(ctx context.Context, enclaveId enclave.EnclaveID, serviceGuid service.ServiceGUID) (*userServiceObjectsAndKubernetesResources, error) {
	searchFilters := &service.ServiceFilters{
		GUIDs: map[service.ServiceGUID]bool{
			serviceGuid: true,
		},
	}
	searchResults, err := backend.getMatchingUserServiceObjectsAndKubernetesResources(ctx, enclaveId, searchFilters)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred finding services matching GUID '%v'", serviceGuid)
	}
	if len(searchResults) == 0 {
		return nil, stacktrace.NewError("No services matched GUID '%v'", serviceGuid)
	}
	if len(searchResults) > 0 {
		return nil, stacktrace.NewError("Expected one service to match GUID '%v' but found %v", serviceGuid, len(searchResults))
	}
	result, found := searchResults[serviceGuid]
	if !found {
		return nil, stacktrace.NewError("Got results from searching for service with GUID '%v', but no results by the GUID we searched for; this is a bug in Kurtosis", serviceGuid)
	}
	return result, nil
}

func (backend *KubernetesKurtosisBackend) getUserServiceKubernetesResourcesMatchingGuids(
	ctx context.Context,
	enclaveId enclave.EnclaveID,
	serviceGuids map[service.ServiceGUID]bool,
) (
	map[service.ServiceGUID]*userServiceKubernetesResources,
	error,
) {
	namespaceName, err := backend.getEnclaveNamespaceName(ctx, enclaveId)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting namespace name for enclave '%v'", enclaveId)
	}

	// TODO switch to properly-typed KubernetesLabelValue object!!!
	postFilterLabelValues := map[string]bool{}
	for serviceGuid := range serviceGuids {
		postFilterLabelValues[string(serviceGuid)] = true
	}

	kubernetesResourceSearchLabels := map[string]string{
		label_key_consts.AppIDKubernetesLabelKey.GetString(): label_value_consts.AppIDKubernetesLabelValue.GetString(),
		label_key_consts.EnclaveIDKubernetesLabelKey.GetString(): string(enclaveId),
		label_key_consts.KurtosisResourceTypeKubernetesLabelKey.GetString(): label_value_consts.UserServiceKurtosisResourceTypeKubernetesLabelValue.GetString(),
	}

	results := map[service.ServiceGUID]*userServiceKubernetesResources{}

	// Get k8s services
	matchingKubernetesServices, err := kubernetes_resource_collectors.CollectMatchingServices(
		ctx,
		backend.kubernetesManager,
		namespaceName,
		kubernetesResourceSearchLabels,
		label_key_consts.GUIDKubernetesLabelKey.GetString(),
		postFilterLabelValues,
	)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting Kubernetes services matching service GUIDs: %+v", serviceGuids)
	}
	for serviceGuidStr, kubernetesServicesForGuid := range matchingKubernetesServices {
		logrus.Tracef("Found Kubernetes services for GUID '%v': %+v", serviceGuidStr, kubernetesServicesForGuid)
		serviceGuid := service.ServiceGUID(serviceGuidStr)

		numServicesForGuid := len(kubernetesServicesForGuid)
		if numServicesForGuid == 0 {
			// This would indicate a bug in our service retrieval logic because we shouldn't even have a map entry if there's nothing matching it
			return nil, stacktrace.NewError("Got entry of result services for service GUID '%v', but no Kubernetes services were returned; this is a bug in Kurtosis", serviceGuid)
		}
		if numServicesForGuid > 1 {
			return nil, stacktrace.NewError("Found %v Kubernetes services associated with service GUID '%v'; this is a bug in Kurtosis", numServicesForGuid, serviceGuid)
		}
		kubernetesService := kubernetesServicesForGuid[0]

		resultObj, found := results[serviceGuid]
		if !found {
			resultObj = &userServiceKubernetesResources{}
		}
		resultObj.service = kubernetesService
		results[serviceGuid] = resultObj
	}

	// Get k8s pods
	matchingKubernetesPods, err := kubernetes_resource_collectors.CollectMatchingPods(
		ctx,
		backend.kubernetesManager,
		namespaceName,
		kubernetesResourceSearchLabels,
		label_key_consts.GUIDKubernetesLabelKey.GetString(),
		postFilterLabelValues,
	)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting Kubernetes pods matching service GUIDs: %+v", serviceGuids)
	}
	for serviceGuidStr, kubernetesPodsForGuid := range matchingKubernetesPods {
		logrus.Tracef("Found Kubernetes pods for GUID '%v': %+v", serviceGuidStr, kubernetesPodsForGuid)
		serviceGuid := service.ServiceGUID(serviceGuidStr)

		numPodsForGuid := len(kubernetesPodsForGuid)
		if numPodsForGuid == 0 {
			// This would indicate a bug in our pod retrieval logic because we shouldn't even have a map entry if there's nothing matching it
			return nil, stacktrace.NewError("Got entry of result pods for service GUID '%v', but no Kubernetes pods were returned; this is a bug in Kurtosis", serviceGuid)
		}
		if numPodsForGuid > 1 {
			return nil, stacktrace.NewError("Found %v Kubernetes pods associated with service GUID '%v'; this is a bug in Kurtosis", numPodsForGuid, serviceGuid)
		}
		kubernetesPod := kubernetesPodsForGuid[0]

		resultObj, found := results[serviceGuid]
		if !found {
			resultObj = &userServiceKubernetesResources{}
		}
		resultObj.pod = kubernetesPod
		results[serviceGuid] = resultObj
	}

	return results, nil
}

func getUserServiceObjectsFromKubernetesResources(
	enclaveId enclave.EnclaveID,
	allKubernetesResources map[service.ServiceGUID]*userServiceKubernetesResources,
) (map[service.ServiceGUID]*userServiceObjectsAndKubernetesResources, error) {
	results := map[service.ServiceGUID]*userServiceObjectsAndKubernetesResources{}
	for serviceGuid, resources := range allKubernetesResources {
		results[serviceGuid] = &userServiceObjectsAndKubernetesResources{
			kubernetesResources: resources,
			// The other fields will get filled in below
		}
	}

	for serviceGuid, resultObj := range results {
		resourcesToParse := resultObj.kubernetesResources

		kubernetesService := resourcesToParse.service
		if kubernetesService == nil {
			return nil, stacktrace.NewError(
				"Service with GUID '%v' doesn't have a Kubernetes service; this indicates either a bug in Kurtosis or that the user manually deleted the Kubernetes service",
				serviceGuid,
			)
		}

		serviceLabels := kubernetesService.Labels
		idLabelStr, found := serviceLabels[label_key_consts.IDKubernetesLabelKey.GetString()]
		if !found {
			return nil, stacktrace.NewError("Expected to find label '%v' on the Kubernetes service but none was found", label_key_consts.IDKubernetesLabelKey.GetString())
		}
		serviceId := service.ServiceID(idLabelStr)

		serviceIpStr := kubernetesService.Spec.ClusterIP
		privateIp := net.ParseIP(serviceIpStr)
		if privateIp == nil {
			return nil, stacktrace.NewError("An error occurred parsing service private IP string '%v' to an IP address object", serviceIpStr)
		}

		serviceRegistrationObj := service.NewServiceRegistration(serviceId, serviceGuid, enclaveId, privateIp)
		resultObj.serviceRegistration = serviceRegistrationObj

		serviceAnnotations := kubernetesService.Annotations
		portSpecsStr, found := serviceAnnotations[annotation_key_consts.PortSpecsAnnotationKey.GetString()]
		if !found {
			// If the service doesn't have a private port specs annotation, it means a pod was never started so there's nothing more to do
			continue
		}
		privatePorts, err := kubernetes_port_spec_serializer.DeserializePortSpecs(portSpecsStr)
		if err != nil {
			return nil, stacktrace.Propagate(err, "An error occurred deserializing port specs string '%v'", portSpecsStr)
		}

		kubernetesPod := resourcesToParse.pod
		if kubernetesPod == nil {
			// No pod here means that a) a Service had private ports but b) now has no Pod
			// This means that there  used to be a Pod but it was stopped/removed
			resultObj.service = service.NewService(
				serviceRegistrationObj,
				container_status.ContainerStatus_Stopped,
				privatePorts,
				servicePublicIp,
				servicePublicPorts,
			)
			continue
		}

		podPhase := kubernetesPod.Status.Phase
		isPodRunning, found := isPodRunningDeterminer[podPhase]
		if !found {
			// Should never happen because we enforce completeness of the determiner via unit test
			return nil, stacktrace.NewError("No is-pod-running determination found for pod phase '%v' on pod '%v'; this is a bug in Kurtosis", podPhase, kubernetesPod.Name)
		}
		// TODO Rename this; this shouldn't be called "ContainerStatus" since there's no longer a 1:1 mapping between container:kurtosis_object
		containerStatus := container_status.ContainerStatus_Stopped
		if isPodRunning {
			containerStatus = container_status.ContainerStatus_Running
		}

		resultObj.service = service.NewService(
			serviceRegistrationObj,
			containerStatus,
			privatePorts,
			servicePublicIp,
			servicePublicPorts,
		)
	}

	return results, nil
}

func getUserServicePodContainerSpecs(
	image string,
	entrypointArgs []string,
	cmdArgs []string,
	envVarStrs map[string]string,
	privatePorts map[string]*port_spec.PortSpec,
	containerMounts []apiv1.VolumeMount,
) (
	[]apiv1.Container,
	error,
){
	var containerEnvVars []apiv1.EnvVar
	for varName, varValue := range envVarStrs {
		envVar := apiv1.EnvVar{
			Name:  varName,
			Value: varValue,
		}
		containerEnvVars = append(containerEnvVars, envVar)
	}

	kubernetesContainerPorts, err := getKubernetesContainerPortsFromPrivatePortSpecs(privatePorts)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred getting Kubernetes container ports from the private port specs map")
	}

	// TODO create networking sidecars here
	containers := []apiv1.Container{
		{
			Name:                     userServiceContainerName,
			Image:                    image,
			// Yes, even though this is called "command" it actually corresponds to the Docker ENTRYPOINT
			Command:                  entrypointArgs,
			Args:                     cmdArgs,
			Ports:                    kubernetesContainerPorts,
			Env:                      containerEnvVars,
			VolumeMounts:             containerMounts,

			// NOTE: There are a bunch of other interesting Container options that we omitted for now but might
			// want to specify in the future
		},
	}

	return containers, nil
}

func (backend *KubernetesKurtosisBackend) getUserServiceVolumeInfoFromFilesArtifactMountpoints(
	ctx context.Context,
	namespaceName string,
	enclaveId enclave.EnclaveID,
	serviceGuid service.ServiceGUID,
	filesArtifactMountpoints map[files_artifact_expansion_volume.FilesArtifactExpansionGUID]string,
) (
	resultPodVolumes []apiv1.Volume,
	resultContainerVolumeMountsOnPod []apiv1.VolumeMount,
	resultErr error,
) {
	filesArtifactExpansionPvcSearchLabels := map[string]string{
		label_key_consts.EnclaveIDKubernetesLabelKey.GetString(): string(enclaveId),
		label_key_consts.KurtosisVolumeTypeKubernetesLabelKey.GetString(): label_value_consts.FilesArtifactExpansionVolumeTypeKubernetesLabelValue.GetString(),
		label_key_consts.UserServiceGUIDKubernetesLabelKey.GetString(): string(serviceGuid),
	}
	persistentVolumeClaimsList, err := backend.kubernetesManager.GetPersistentVolumeClaimsByLabels(ctx, namespaceName, filesArtifactExpansionPvcSearchLabels)
	if err != nil {
		return nil, nil, stacktrace.Propagate(err, "An error occurred getting files artifact expansion persistent volume claims for service '%v' matching labels: %+v", serviceGuid, filesArtifactExpansionPvcSearchLabels)
	}
	persistentVolumeClaims := persistentVolumeClaimsList.Items

	if len(persistentVolumeClaims) != len(filesArtifactMountpoints) {
		return nil, nil, stacktrace.Propagate(
			err,
			"Received request to start service with %v files artifact mountpoints '%+v', but only found %v persistent volume claims prepared for the service",
			len(filesArtifactMountpoints),
			filesArtifactMountpoints,
			len(persistentVolumeClaims),
		)
	}

	// Index PVCs by files artifact expansion GUID
	persistentVolumeClaimsByExpansionGuid := map[files_artifact_expansion_volume.FilesArtifactExpansionGUID]*apiv1.PersistentVolumeClaim{}
	for _, claim := range persistentVolumeClaims {
		expansionGuidStr, found := claim.Labels[label_key_consts.GUIDKubernetesLabelKey.GetString()]
		if !found {
			return nil, nil, stacktrace.NewError(
				"Files artifact expansion persistent volume claim is missing required label '%v'",
				label_key_consts.GUIDKubernetesLabelKey.GetString(),
			)
		}
		expansionGuid := files_artifact_expansion_volume.FilesArtifactExpansionGUID(expansionGuidStr)
		persistentVolumeClaimsByExpansionGuid[expansionGuid] = &claim
	}

	podVolumes := []apiv1.Volume{}
	containerMounts := []apiv1.VolumeMount{}
	for expansionGuid, mountpoint := range filesArtifactMountpoints {
		claim, found := persistentVolumeClaimsByExpansionGuid[expansionGuid]
		if !found {
			return nil, nil, stacktrace.NewError("Expected a persistent volume claim for files artifact expansion '%v' but none was found", expansionGuid)
		}
		claimName := claim.Name
		volumeName := claim.Spec.VolumeName

		/*
		claimName := claim.Name

		filesArtifactId, found := claim.Labels[label_key_consts.FilesArtifactIDKubernetesLabelKey.GetString()]
		if !found {
			return nil, nil, stacktrace.NewError(
				"Files artifact expansion persistent volume claim '%v' didn't have expected label '%v'",
				claimName,
				label_key_consts.FilesArtifactIDKubernetesLabelKey.GetString(),
			)
		}
		 */


		volumeObj := apiv1.Volume{
			Name:         volumeName,
			VolumeSource: apiv1.VolumeSource{
				PersistentVolumeClaim: &apiv1.PersistentVolumeClaimVolumeSource{
					ClaimName: claimName,
				},
			},
		}
		podVolumes = append(podVolumes, volumeObj)

		containerMountObj := apiv1.VolumeMount{
			Name:             volumeName,
			ReadOnly:         shouldMountVolumesAsReadOnly,
			MountPath:        mountpoint,
		}
		containerMounts = append(containerMounts, containerMountObj)
	}

	return podVolumes, containerMounts, nil
}

// Update the service to:
// - Set the service ports appropriately
// - Irrevocably record that a pod is bound to the service (so that even if the pod is deleted, the service won't
// 	 be usable again
func (backend *KubernetesKurtosisBackend) updateServiceWhenContainerStarted(
	ctx context.Context,
	namespaceName string,
	kubernetesService *apiv1.Service,
	privatePorts map[string]*port_spec.PortSpec,
) (*apiv1.Service, error) {
	updatedService := kubernetesService.DeepCopy()

	serializedPortSpecs, err := kubernetes_port_spec_serializer.SerializePortSpecs(privatePorts)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred serializing the following private port specs: %+v", privatePorts)
	}

	// We only need to modify the ports from the default (unbound) ports if the user actually declares ports
	if len(privatePorts) > 0 {
		kubernetesServicePorts, err := getKubernetesServicePortsFromPrivatePortSpecs(privatePorts)
		if err != nil {
			return nil, stacktrace.Propagate(err, "An error occurred getting Kubernetes service ports for the following private port specs: %+v", privatePorts)
		}
		updatedService.Spec.Ports = kubernetesServicePorts
	}

	updatedAnnotations := updatedService.Annotations
	if updatedAnnotations == nil {
		updatedAnnotations = map[string]string{}
	}
	updatedAnnotations[annotation_key_consts.PortSpecsAnnotationKey.GetString()] = serializedPortSpecs.GetString()
	updatedService.Annotations = updatedAnnotations

	if err := backend.kubernetesManager.UpdateService(ctx, namespaceName, updatedService); err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred updating the user service to open the service ports and add the serialized private port specs annotation")
	}
	return updatedService, nil
}