package add_service

import (
	"context"
	"fmt"
	"github.com/kurtosis-tech/kurtosis/api/golang/core/kurtosis_core_rpc_api_bindings"
	"github.com/kurtosis-tech/kurtosis/api/golang/core/lib/binding_constructors"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/service"
	kurtosis_backend_service "github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/service"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/service_network"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_instruction"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_instruction/shared_helpers"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_instruction/shared_helpers/magic_string_helper"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_types"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/runtime_value_store"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/startosis_errors"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/startosis_validator"
	"github.com/kurtosis-tech/kurtosis/core/server/commons/enclave_data_directory"
	"github.com/kurtosis-tech/stacktrace"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

const (
	AddServiceBuiltinName = "add_service"

	serviceIdArgName = "service_id"

	serviceConfigArgName = "config"
)

func GenerateAddServiceBuiltin(instructionsQueue *[]kurtosis_instruction.KurtosisInstruction, serviceNetwork service_network.ServiceNetwork, runtimeValueStore *runtime_value_store.RuntimeValueStore) func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	// TODO: Force returning an InterpretationError rather than a normal error
	return func(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		instructionPosition := shared_helpers.GetCallerPositionFromThread(thread)
		addServiceInstruction := newEmptyAddServiceInstruction(serviceNetwork, instructionPosition, runtimeValueStore)
		if interpretationError := addServiceInstruction.parseStartosisArgs(b, args, kwargs); interpretationError != nil {
			return nil, interpretationError
		}
		*instructionsQueue = append(*instructionsQueue, addServiceInstruction)
		returnValue, interpretationError := addServiceInstruction.makeAddServiceInterpretationReturnValue()
		if interpretationError != nil {
			return nil, interpretationError
		}
		return returnValue, nil
	}
}

type AddServiceInstruction struct {
	serviceNetwork    service_network.ServiceNetwork
	runtimeValueStore *runtime_value_store.RuntimeValueStore

	position       *kurtosis_instruction.InstructionPosition
	starlarkKwargs starlark.StringDict

	serviceId     kurtosis_backend_service.ServiceID
	serviceConfig *kurtosis_core_rpc_api_bindings.ServiceConfig
}

func newEmptyAddServiceInstruction(serviceNetwork service_network.ServiceNetwork, position *kurtosis_instruction.InstructionPosition, runtimeValueStore *runtime_value_store.RuntimeValueStore) *AddServiceInstruction {
	return &AddServiceInstruction{
		serviceNetwork:    serviceNetwork,
		position:          position,
		starlarkKwargs:    starlark.StringDict{},
		serviceId:         "",
		serviceConfig:     nil,
		runtimeValueStore: runtimeValueStore,
	}
}

func NewAddServiceInstruction(serviceNetwork service_network.ServiceNetwork, position *kurtosis_instruction.InstructionPosition, serviceId kurtosis_backend_service.ServiceID, serviceConfig *kurtosis_core_rpc_api_bindings.ServiceConfig, starlarkKwargs starlark.StringDict, runtimeValueStore *runtime_value_store.RuntimeValueStore) *AddServiceInstruction {
	return &AddServiceInstruction{
		serviceNetwork:    serviceNetwork,
		position:          position,
		serviceId:         serviceId,
		serviceConfig:     serviceConfig,
		starlarkKwargs:    starlarkKwargs,
		runtimeValueStore: runtimeValueStore,
	}
}

func (instruction *AddServiceInstruction) GetPositionInOriginalScript() *kurtosis_instruction.InstructionPosition {
	return instruction.position
}

func (instruction *AddServiceInstruction) GetCanonicalInstruction() *kurtosis_core_rpc_api_bindings.StarlarkInstruction {
	args := []*kurtosis_core_rpc_api_bindings.StarlarkInstructionArg{
		binding_constructors.NewStarlarkInstructionKwarg(shared_helpers.CanonicalizeArgValue(instruction.starlarkKwargs[serviceIdArgName]), serviceIdArgName, kurtosis_instruction.Representative),
		binding_constructors.NewStarlarkInstructionKwarg(shared_helpers.CanonicalizeArgValue(instruction.starlarkKwargs[serviceConfigArgName]), serviceConfigArgName, kurtosis_instruction.NotRepresentative),
	}
	return binding_constructors.NewStarlarkInstruction(instruction.position.ToAPIType(), AddServiceBuiltinName, instruction.String(), args)
}

func (instruction *AddServiceInstruction) Execute(ctx context.Context) (*string, error) {
	serviceIdStr, err := magic_string_helper.ReplaceRuntimeValueInString(string(instruction.serviceId), instruction.runtimeValueStore)
	if err != nil {
		return nil, stacktrace.Propagate(err, "Error occurred while replacing facts in service id for '%v'", instruction.serviceId)
	}
	instruction.serviceId = kurtosis_backend_service.ServiceID(serviceIdStr)
	err = instruction.replaceMagicStrings()
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred replacing IP Address with actual values in add service instruction for service '%v'", instruction.serviceId)
	}

	serviceConfigMap := map[service.ServiceID]*kurtosis_core_rpc_api_bindings.ServiceConfig{
		instruction.serviceId: instruction.serviceConfig,
	}

	serviceSuccessful, serviceFailed, err := instruction.serviceNetwork.StartServices(ctx, serviceConfigMap)
	if err != nil {
		return nil, stacktrace.Propagate(err, "Failed adding service to enclave with an unexpected error")
	}
	if failure, found := serviceFailed[instruction.serviceId]; found {
		return nil, stacktrace.Propagate(failure, "Failed adding service to enclave")
	}
	deployedService, found := serviceSuccessful[instruction.serviceId]
	if !found {
		return nil, stacktrace.NewError("Service wasn't accounted as failed nor successfully added. This is a product bug")
	}
	instructionResult := fmt.Sprintf("Service '%s' added with service GUID '%s'", instruction.serviceId, deployedService.GetRegistration().GetGUID())
	return &instructionResult, nil
}

func (instruction *AddServiceInstruction) String() string {
	return shared_helpers.CanonicalizeInstruction(AddServiceBuiltinName, kurtosis_instruction.NoArgs, instruction.starlarkKwargs)
}

func (instruction *AddServiceInstruction) replaceMagicStrings() error {
	serviceIdStr := string(instruction.serviceId)
	entryPointArgs := instruction.serviceConfig.EntrypointArgs
	for index, entryPointArg := range entryPointArgs {
		entryPointArgWithIPAddressReplaced, err := magic_string_helper.ReplaceIPAddressInString(entryPointArg, instruction.serviceNetwork, serviceIdStr)
		if err != nil {
			return stacktrace.Propagate(err, "Error occurred while replacing IP address in entry point args for '%v'", entryPointArg)
		}
		entryPointArgWithIPAddressAndRuntimeValueReplaced, err := magic_string_helper.ReplaceRuntimeValueInString(entryPointArgWithIPAddressReplaced, instruction.runtimeValueStore)
		if err != nil {
			return stacktrace.Propagate(err, "Error occurred while replacing runtime value in entry point args for '%v'", entryPointArg)
		}
		entryPointArgs[index] = entryPointArgWithIPAddressAndRuntimeValueReplaced
	}

	cmdArgs := instruction.serviceConfig.CmdArgs
	for index, cmdArg := range cmdArgs {
		cmdArgWithIPAddressReplaced, err := magic_string_helper.ReplaceIPAddressInString(cmdArg, instruction.serviceNetwork, serviceIdStr)
		if err != nil {
			return stacktrace.Propagate(err, "Error occurred while replacing IP address in command args for '%v'", cmdArg)
		}
		cmdArgWithIPAddressAndRuntimeValueReplaced, err := magic_string_helper.ReplaceRuntimeValueInString(cmdArgWithIPAddressReplaced, instruction.runtimeValueStore)
		if err != nil {
			return stacktrace.Propagate(err, "Error occurred while replacing runtime value in command args for '%v'", cmdArg)
		}
		cmdArgs[index] = cmdArgWithIPAddressAndRuntimeValueReplaced
	}

	envVars := instruction.serviceConfig.EnvVars
	for envVarName, envVarValue := range envVars {
		envVarValueWithIPAddressReplaced, err := magic_string_helper.ReplaceIPAddressInString(envVarValue, instruction.serviceNetwork, serviceIdStr)
		if err != nil {
			return stacktrace.Propagate(err, "Error occurred while replacing IP address in env vars for '%v'", envVarValue)
		}
		envVarValueWithIPAddressAndRuntimeValueReplaced, err := magic_string_helper.ReplaceRuntimeValueInString(envVarValueWithIPAddressReplaced, instruction.runtimeValueStore)
		if err != nil {
			return stacktrace.Propagate(err, "Error occurred while replacing runtime value in command args for '%v'", envVars)
		}
		envVars[envVarName] = envVarValueWithIPAddressAndRuntimeValueReplaced
	}

	return nil
}

func (instruction *AddServiceInstruction) makeAddServiceInterpretationReturnValue() (*kurtosis_types.Service, *startosis_errors.InterpretationError) {
	ports := instruction.serviceConfig.GetPrivatePorts()
	portSpecsDict := starlark.NewDict(len(ports))
	for portId, port := range ports {
		number := port.GetNumber()
		transportProtocol := port.GetTransportProtocol()
		maybeApplicationProtocol := port.GetMaybeApplicationProtocol()

		portSpec := kurtosis_types.NewPortSpec(number, transportProtocol, maybeApplicationProtocol)
		if err := portSpecsDict.SetKey(starlark.String(portId), portSpec); err != nil {
			return nil, startosis_errors.NewInterpretationError("An error occurred while creating a port spec for values "+
				"(number: '%v', transport_protocol: '%v', application_protocol: '%v') the add instruction return value",
				number, transportProtocol, maybeApplicationProtocol)
		}
	}
	ipAddress := starlark.String(fmt.Sprintf(magic_string_helper.IpAddressReplacementPlaceholderFormat, instruction.serviceId))
	returnValue := kurtosis_types.NewService(ipAddress, portSpecsDict)
	return returnValue, nil
}

func (instruction *AddServiceInstruction) ValidateAndUpdateEnvironment(environment *startosis_validator.ValidatorEnvironment) error {
	if environment.DoesServiceIdExist(instruction.serviceId) {
		return startosis_errors.NewValidationError("There was an error validating '%v' as service ID '%v' already exists", AddServiceBuiltinName, instruction.serviceId)
	}
	for artifactIdKey := range instruction.serviceConfig.FilesArtifactMountpoints {
		if !environment.DoesArtifactIdExist(enclave_data_directory.FilesArtifactID(artifactIdKey)) {
			return startosis_errors.NewValidationError("There was an error validating '%v' as artifact UUID '%v' does not exist", AddServiceBuiltinName, artifactIdKey)
		}
	}
	environment.AddServiceId(instruction.serviceId)
	environment.AppendRequiredContainerImage(instruction.serviceConfig.ContainerImageName)
	return nil
}

func (instruction *AddServiceInstruction) parseStartosisArgs(b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) *startosis_errors.InterpretationError {
	// TODO(gb): Right now, we expect the Startosis script to be very "untyped" like:
	//  ```startosis
	//  my_service_port = struct(port = 1234, protocol = "TCP")
	//  my_config = struct(private_port = port, other_arg = "blah")
	//  ```
	//  But we can do better than this defining our own structures:
	//  ```
	//  my_service_port = port_spec(port = 1234, protocol = "TCP") # port() is a Startosis defined struct
	//  my_config = config(port = port, other_arg = "blah")
	//  ```
	//  With custom types, we can parse the args directly to our own Go types and potentially isolate the checks

	var serviceIdArg starlark.String
	var serviceConfigArg *starlarkstruct.Struct
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, serviceIdArgName, &serviceIdArg, serviceConfigArgName, &serviceConfigArg); err != nil {
		return startosis_errors.WrapWithInterpretationError(err, "Failed parsing arguments for function '%s' (unparsed arguments were: '%v' '%v')", AddServiceBuiltinName, args, kwargs)
	}
	instruction.starlarkKwargs[serviceIdArgName] = serviceIdArg
	instruction.starlarkKwargs[serviceConfigArgName] = serviceConfigArg
	instruction.starlarkKwargs.Freeze()

	serviceId, interpretationErr := kurtosis_instruction.ParseServiceId(serviceIdArg)
	if interpretationErr != nil {
		return interpretationErr
	}

	serviceConfig, interpretationErr := kurtosis_instruction.ParseServiceConfigArg(serviceConfigArg)
	if interpretationErr != nil {
		return interpretationErr
	}
	instruction.serviceId = serviceId
	instruction.serviceConfig = serviceConfig
	return nil
}
