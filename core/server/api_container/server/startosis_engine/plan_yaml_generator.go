package startosis_engine

import (
	"github.com/go-yaml/yaml"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/service_network"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/instructions_plan"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_instruction/add_service"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_instruction/remove_service"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_instruction/render_templates"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_instruction/tasks"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_instruction/upload_files"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_starlark_framework/builtin_argument"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_types"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/kurtosis_types/service_config"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/startosis_errors"
	"github.com/kurtosis-tech/kurtosis/core/server/api_container/server/startosis_engine/startosis_packages"
	"go.starlark.net/starlark"
)

// We need the package id and the args, the args need to be filled in
// the instructions likely come with the args filled in already, but what if no args are passed in? are they left as variables?

// How to represent dependencies within the yaml???
// say a service config refers to another files artifact

// some conversions are:
// add_service -> use service config and returned info to create a ServiceObject
// remove_service -> remove that from the plan representation
// upload_files -> FilesArtifact
// render_template -> FilesArtifact
// run_sh -> Task but returns a files artifact so create that
// run_python -> Task but returns a files artifact so create that
//
// go through all the kurtosis builtins and figure out which ones we need to accommodate for and which ones we don't need to accommodate for

// TODO: refactor this so plan yaml is generated as instructionsPlan is created, otherwise a lot of duplicate operations happen to parse the arguments

// PlanYamlGenerator generates a yaml representation of a [plan].
type PlanYamlGenerator interface {
	// GenerateYaml converts [plan] into a byte array that represents a yaml with information in the plan.
	// The format of the yaml in the byte array is as such:
	//
	//
	//
	// packageId: github.com/kurtosis-tech/postgres-package
	//
	// services:
	// 	- uuid:
	//	- name:
	//    service_config:
	//	  	image:
	//		env_var:
	//		...
	//
	//
	// files_artifacts:
	//
	//
	//
	//
	//
	//
	// tasks:
	//
	//

	GenerateYaml(plan instructions_plan.InstructionsPlan) ([]byte, error)
}

type PlanYamlGeneratorImpl struct {
	// Plan generated by an interpretation of a Starlark script of package
	plan *instructions_plan.InstructionsPlan

	serviceNetwork service_network.ServiceNetwork

	packageContentProvider startosis_packages.PackageContentProvider

	locatorOfModuleInWhichThisBuiltInIsBeingCalled string

	packageReplaceOptions map[string]string

	// stores all future references returned from instructions so they can be referenced later
	futureReferencesStore map[string]string

	// technically files artifacts are future references but we store them separately bc they are easily identifiable
	// and have a distinct structure (FilesArtifact)
	filesArtifactIndex map[string]*FilesArtifact

	// Store service index needed to see in case a service is referenced by a remove service, or store service later in the plan
	serviceIndex map[string]*Service

	// TODO: do we need a task index?
	taskIndex map[string]*Task

	// Representation of plan in yaml the plan is being processed, the yaml gets updated
	planYaml *PlanYaml
}

func NewPlanYamlGenerator(
	plan *instructions_plan.InstructionsPlan,
	serviceNetwork service_network.ServiceNetwork,
	packageId string,
	packageContentProvider startosis_packages.PackageContentProvider,
	locatorOfModuleInWhichThisBuiltInIsBeingCalled string,
	packageReplaceOptions map[string]string) *PlanYamlGeneratorImpl {
	return &PlanYamlGeneratorImpl{
		plan:                   plan,
		serviceNetwork:         serviceNetwork,
		packageContentProvider: packageContentProvider,
		packageReplaceOptions:  packageReplaceOptions,
		locatorOfModuleInWhichThisBuiltInIsBeingCalled: locatorOfModuleInWhichThisBuiltInIsBeingCalled,
		planYaml: &PlanYaml{
			PackageId: packageId,
		},
		filesArtifactIndex: map[string]*FilesArtifact{},
		serviceIndex:       map[string]*Service{},
		taskIndex:          map[string]*Task{},
	}
}

func (pyg *PlanYamlGeneratorImpl) GenerateYaml() ([]byte, error) {
	instructionsSequence, err := pyg.plan.GeneratePlan()
	if err != nil {
		return nil, err
	}

	// iterate over the sequence of instructions
	for _, scheduledInstruction := range instructionsSequence {
		var err error
		// based on the instruction, update the plan yaml representation accordingly
		switch getBuiltinNameFromInstruction(scheduledInstruction) {
		case add_service.AddServiceBuiltinName:
			err = pyg.updatePlanYamlFromAddService(scheduledInstruction)
		case remove_service.RemoveServiceBuiltinName:
			err = pyg.updatePlanYamlFromRemoveService(scheduledInstruction)
		case tasks.RunShBuiltinName:
			err = pyg.updatePlanYamlFromRunSh(scheduledInstruction)
		case tasks.RunPythonBuiltinName:
			err = pyg.updatePlanYamlFromRunPython(scheduledInstruction)
		case render_templates.RenderTemplatesBuiltinName:
			err = pyg.updatePlanYamlFromRenderTemplates(scheduledInstruction)
		case upload_files.UploadFilesBuiltinName:
			err = pyg.updatePlanYamlFromUploadFiles(scheduledInstruction)
		default:
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
	}

	// at the very end, convert the plan yaml representation into a yaml
	return convertPlanYamlToYaml(pyg.planYaml)
}

func (pyg *PlanYamlGeneratorImpl) updatePlanYamlFromAddService(addServiceInstruction *instructions_plan.ScheduledInstruction) error { // for type safety, it would be great to be more specific than scheduled instruction
	kurtosisInstruction := addServiceInstruction.GetInstruction()
	arguments := kurtosisInstruction.GetArguments()

	// start building Service Yaml object
	service := &Service{}
	service.Uuid = string(addServiceInstruction.GetUuid()) // TODO: mock uuid generator for testing

	serviceName, err := builtin_argument.ExtractArgumentValue[starlark.String](arguments, add_service.ServiceNameArgName)
	if err != nil {
		return startosis_errors.WrapWithInterpretationError(err, "Unable to extract value for '%s' argument", add_service.ServiceNameArgName)
	}
	service.Name = string(serviceName)

	starlarkServiceConfig, err := builtin_argument.ExtractArgumentValue[*service_config.ServiceConfig](arguments, add_service.ServiceConfigArgName)
	if err != nil {
		return startosis_errors.WrapWithInterpretationError(err, "Unable to extract value for '%s' argument", add_service.ServiceConfigArgName)
	}
	serviceConfig, err := starlarkServiceConfig.ToKurtosisType( // is this an expensive call?
		pyg.serviceNetwork,
		pyg.locatorOfModuleInWhichThisBuiltInIsBeingCalled,
		pyg.planYaml.PackageId,
		pyg.packageContentProvider,
		pyg.packageReplaceOptions)

	service.Image = serviceConfig.GetContainerImageName() // TODO: support image build specs, image registry specs, nix build specs
	service.Cmd = serviceConfig.GetCmdArgs()
	service.Entrypoint = serviceConfig.GetEntrypointArgs()

	// ports
	service.Ports = []*Port{}
	for portName, configPort := range serviceConfig.GetPrivatePorts() { // TODO: support public ports
		port := &Port{
			TransportProtocol:   TransportProtocol(configPort.GetTransportProtocol().String()),
			ApplicationProtocol: ApplicationProtocol(*configPort.GetMaybeApplicationProtocol()),
			Name:                portName,
			Number:              configPort.GetNumber(),
		}
		service.Ports = append(service.Ports, port)
	}

	// env vars
	service.EnvVars = []*EnvironmentVariable{}
	for key, val := range serviceConfig.GetEnvVars() {
		envVar := &EnvironmentVariable{
			Key:   key,
			Value: val,
		}
		service.EnvVars = append(service.EnvVars, envVar)
	}

	// file mounts have two cases:
	// 1. the referenced files artifact already exists in the plan, in which case add the referenced files artifact
	// 2. the referenced files artifact does not already exist in the plan, in which case the file MUST have been passed in via a top level arg OR is invalid
	// 	  in this case,
	// 	  - create new files artifact
	//	  - add it to the service's file mount accordingly
	//	  - add the files artifact to the plan
	service.Files = []*FileMount{}
	serviceFilesArtifactExpansions := serviceConfig.GetFilesArtifactsExpansion()
	if serviceFilesArtifactExpansions != nil {
		for mountPath, artifactIdentifiers := range serviceFilesArtifactExpansions.ServiceDirpathsToArtifactIdentifiers {
			var fileMount *FileMount
			fileMount = &FileMount{
				MountPath: mountPath,
			}

			var serviceFilesArtifacts []*FilesArtifact
			for _, identifier := range artifactIdentifiers {
				var filesArtifact *FilesArtifact
				// if there's already a files artifact that exists with this name from a previous instruction, reference that
				if potentialFilesArtifact, ok := pyg.filesArtifactIndex[identifier]; ok {
					filesArtifact = &FilesArtifact{
						Uuid: potentialFilesArtifact.Uuid,
						Name: potentialFilesArtifact.Name,
					}
				} else {
					// otherwise create a new one
					// the only information we have about a files artifact that didn't already exist is the name
					// if it didn't already exist AND interpretation was successful, it MUST HAVE been passed in via args
					filesArtifact = &FilesArtifact{
						Name: identifier,
					}
					pyg.planYaml.FilesArtifacts = append(pyg.planYaml.FilesArtifacts, filesArtifact)
					pyg.filesArtifactIndex[identifier] = filesArtifact
				}
				serviceFilesArtifacts = append(serviceFilesArtifacts, filesArtifact)
			}

			fileMount.FilesArtifacts = serviceFilesArtifacts
			service.Files = append(service.Files, fileMount)
		}

	}

	pyg.planYaml.Services = append(pyg.planYaml.Services, service)
	pyg.serviceIndex[service.Name] = service
	return nil
}

func (pyg *PlanYamlGeneratorImpl) updatePlanYamlFromUploadFiles(uploadFilesInstruction *instructions_plan.ScheduledInstruction) error {
	var filesArtifact *FilesArtifact

	// get the name of returned files artifact
	filesArtifactName, castErr := kurtosis_types.SafeCastToString(uploadFilesInstruction.GetReturnedValue(), "files artifact name")
	if castErr != nil {
		return castErr
	}
	filesArtifact = &FilesArtifact{
		Uuid: string(uploadFilesInstruction.GetUuid()), // give the FilesArtifact the uuid of the originating instruction
		Name: filesArtifactName,
	}

	// get files of returned files artifact off render templates config
	arguments := uploadFilesInstruction.GetInstruction().GetArguments()
	src, err := builtin_argument.ExtractArgumentValue[starlark.String](arguments, upload_files.SrcArgName)
	if err != nil {
		return startosis_errors.WrapWithInterpretationError(err, "Unable to extract value for '%s' argument", upload_files.SrcArgName)
	}
	filesArtifact.Files = []string{src.GoString()}

	// add the files artifact to the yaml and index
	pyg.planYaml.FilesArtifacts = append(pyg.planYaml.FilesArtifacts, filesArtifact)
	pyg.filesArtifactIndex[filesArtifactName] = filesArtifact
	return nil
}

func (pyg *PlanYamlGeneratorImpl) updatePlanYamlFromRenderTemplates(renderTemplatesInstruction *instructions_plan.ScheduledInstruction) error {
	var filesArtifact *FilesArtifact

	// get the name of returned files artifact
	filesArtifactName, castErr := kurtosis_types.SafeCastToString(renderTemplatesInstruction.GetReturnedValue(), "files artifact name")
	if castErr != nil {
		return castErr
	}
	filesArtifact = &FilesArtifact{
		Uuid: string(renderTemplatesInstruction.GetUuid()), // give the FilesArtifact the uuid of the originating instruction
		Name: filesArtifactName,
	}

	// get files of returned files artifact off render templates config
	arguments := renderTemplatesInstruction.GetInstruction().GetArguments()
	renderTemplateConfig, err := builtin_argument.ExtractArgumentValue[*starlark.Dict](arguments, render_templates.TemplateAndDataByDestinationRelFilepathArg)
	if err != nil {
		return startosis_errors.WrapWithInterpretationError(err, "Unable to parse '%s'", render_templates.TemplateAndDataByDestinationRelFilepathArg)
	}
	files := []string{}
	for _, filepath := range renderTemplateConfig.Keys() {
		filepathStr, castErr := kurtosis_types.SafeCastToString(filepath, "filepath")
		if castErr != nil {
			return castErr
		}
		files = append(files, filepathStr)
	}
	filesArtifact.Files = files

	// add the files artifact to the yaml and index
	pyg.planYaml.FilesArtifacts = append(pyg.planYaml.FilesArtifacts, filesArtifact)
	pyg.filesArtifactIndex[filesArtifactName] = filesArtifact

	return nil
}

func (pyg *PlanYamlGeneratorImpl) updatePlanYamlFromRunSh(runShInstruction *instructions_plan.ScheduledInstruction) error {
	var task *Task

	// set instruction uuid
	instructionUuid := string(runShInstruction.GetUuid())
	// get the name of
	//runShInstructionName, castErr := kurtosis_types.SafeCastToString(runShInstruction.GetInstruction(), "")
	//if castErr != nil {
	//	return castErr
	//}
	task = &Task{
		Uuid:     instructionUuid,
		TaskType: SHELL,
	}

	// get runcmd, image, env vars and set them in the yaml
	arguments := runShInstruction.GetInstruction().GetArguments()
	runCommand, err := builtin_argument.ExtractArgumentValue[starlark.String](arguments, tasks.RunArgName)
	if err != nil {
		return startosis_errors.WrapWithInterpretationError(err, "Unable to extract value for '%s' argument", tasks.RunArgName)
	}
	task.RunCmd = runCommand.GoString()

	var image string
	if arguments.IsSet(tasks.ImageNameArgName) {
		imageStarlark, err := builtin_argument.ExtractArgumentValue[starlark.String](arguments, tasks.ImageNameArgName)
		if err != nil {
			return startosis_errors.WrapWithInterpretationError(err, "Unable to extract value for '%s' argument", tasks.ImageNameArgName)
		}
		image = imageStarlark.GoString()
	} else {
		image = tasks.DefaultRunShImageName
	}
	task.Image = image

	var envVars []*EnvironmentVariable
	var envVarsMap map[string]string
	if arguments.IsSet(tasks.EnvVarsArgName) {
		envVarsStarlark, err := builtin_argument.ExtractArgumentValue[*starlark.Dict](arguments, tasks.EnvVarsArgName)
		if err != nil {
			return startosis_errors.WrapWithInterpretationError(err, "Unable to extract value for '%s' argument", tasks.EnvVarsArgName)
		}
		if envVarsStarlark != nil && envVarsStarlark.Len() > 0 {
			var interpretationErr *startosis_errors.InterpretationError
			envVarsMap, interpretationErr = kurtosis_types.SafeCastToMapStringString(envVarsStarlark, tasks.EnvVarsArgName)
			if interpretationErr != nil {
				return interpretationErr
			}
		}
	}
	for key, val := range envVarsMap {
		envVars = append(envVars, &EnvironmentVariable{
			Key:   key,
			Value: val,
		})
	}
	task.EnvVars = envVars

	// for files:
	//	1. either the referenced files artifact already exists in the plan, in which case, look for it and reference it via instruction uuid
	// 	2. the referenced files artifact is new, in which case we add it to the plan
	if arguments.IsSet(tasks.FilesArgName) {
		filesStarlark, err := builtin_argument.ExtractArgumentValue[*starlark.Dict](arguments, tasks.FilesArgName)
		if err != nil {
			return startosis_errors.WrapWithInterpretationError(err, "Unable to extract value for '%s' argument", tasks.FilesArgName)
		}
		if filesStarlark.Len() > 0 {
			filesArtifactMountDirPaths, interpretationErr := kurtosis_types.SafeCastToMapStringString(filesStarlark, tasks.FilesArgName)
			if interpretationErr != nil {
				return interpretationErr
			}
			for mountPath, fileArtifactName := range filesArtifactMountDirPaths {
				var filesArtifact *FilesArtifact
				// if there's already a files artifact that exists with this name from a previous instruction, reference that
				if potentialFilesArtifact, ok := pyg.filesArtifactIndex[fileArtifactName]; ok {
					filesArtifact = &FilesArtifact{
						Uuid: potentialFilesArtifact.Uuid,
						Name: potentialFilesArtifact.Name,
					}
				} else {
					// otherwise create a new one
					// the only information we have about a files artifact that didn't already exist is the name
					// if it didn't already exist AND interpretation was successful, it MUST HAVE been passed in via args
					filesArtifact = &FilesArtifact{
						Name: fileArtifactName,
					}
					// add to the index and append to the plan yaml
					pyg.planYaml.FilesArtifacts = append(pyg.planYaml.FilesArtifacts, filesArtifact)
					pyg.filesArtifactIndex[fileArtifactName] = filesArtifact
				}

				task.Files = append(task.Files, &FileMount{
					MountPath:      mountPath,
					FilesArtifacts: []*FilesArtifact{filesArtifact},
				})
			}
		}
	}

	// for store
	// - all files artifacts product from store are new files artifact that are added to the plan
	//		- add them to files artifacts list
	// 		- add them to the store section of run sh
	var store []*FilesArtifact
	storeSpecs, err := tasks.ParseStoreFilesArg(pyg.serviceNetwork, arguments)
	// TODO: catch this error
	//if err != startosis_errors.WrapWithInterpretationError(nil, "") { catch this error
	//	return err
	//}
	for _, storeSpec := range storeSpecs {
		// add the FilesArtifact to list of all files artifacts and index
		var newFilesArtifactFromStoreSpec = &FilesArtifact{
			Uuid:  instructionUuid,
			Name:  storeSpec.GetName(),
			Files: []string{storeSpec.GetSrc()},
		}
		pyg.filesArtifactIndex[storeSpec.GetName()] = newFilesArtifactFromStoreSpec
		pyg.planYaml.FilesArtifacts = append(pyg.planYaml.FilesArtifacts, newFilesArtifactFromStoreSpec)
		store = append(store, &FilesArtifact{
			Name:  storeSpec.GetName(),
			Files: []string{storeSpec.GetSrc()},
		})
	}
	task.Store = store // TODO: be consistent about how I'm setting lists in the plan yamls, probably should add wrappers to the plan yaml

	// add task to index, do we even need a tasks index?
	pyg.planYaml.Tasks = append(pyg.planYaml.Tasks, task)
	return nil
}

func (pyg *PlanYamlGeneratorImpl) updatePlanYamlFromRunPython(runPythonInstruction *instructions_plan.ScheduledInstruction) error {
	panic("run python not implemented yet")
	return nil
	// TODO: update the plan yaml based on an add_service
}

func (pyg *PlanYamlGeneratorImpl) updatePlanYamlFromStoreService(storeServiceInstruction *instructions_plan.ScheduledInstruction) error {
	panic("store service not implemented yet")
	return nil
}

func (pyg *PlanYamlGeneratorImpl) updatePlanYamlFromRemoveService(RemoveServiceInstruction *instructions_plan.ScheduledInstruction) error {
	panic("remove service not implemented yet")
	return nil
	// TODO: update the plan yaml based on an add_service
}

func convertPlanYamlToYaml(planYaml *PlanYaml) ([]byte, error) {
	// unravel all the indices and add them to the plan
	// add some sort of tie breaking so yaml's are deterministic

	yamlBytes, err := yaml.Marshal(planYaml)
	if err != nil {
		return []byte{}, err
	}
	return yamlBytes, nil
}

func getBuiltinNameFromInstruction(instruction *instructions_plan.ScheduledInstruction) string {
	return instruction.GetInstruction().GetCanonicalInstruction(false).GetInstructionName()
}
