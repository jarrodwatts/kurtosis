package startosis_validator

import "github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/service"

// ValidatorEnvironment fields are not exported so that only validators can access its fields
type ValidatorEnvironment struct {
	requiredDockerImages map[string]bool
	serviceIDs           map[service.ServiceID]bool
}

func NewValidatorEnvironment(serviceIDs map[service.ServiceID]bool) *ValidatorEnvironment {
	return &ValidatorEnvironment{
		requiredDockerImages: map[string]bool{},
		serviceIDs:           serviceIDs,
	}
}

func (environment *ValidatorEnvironment) AppendRequiredDockerImage(dockerImage string) {
	environment.requiredDockerImages[dockerImage] = true
}

func (environment *ValidatorEnvironment) AddServiceId(serviceId service.ServiceID) {
	environment.serviceIDs[serviceId] = true
}

func (environment *ValidatorEnvironment) RemoveServiceId(serviceId service.ServiceID) {
	delete(environment.serviceIDs, serviceId)
}

func (environment *ValidatorEnvironment) DoesServiceIdExist(serviceId service.ServiceID) bool {
	_, ok := environment.serviceIDs[serviceId]
	return ok
}
