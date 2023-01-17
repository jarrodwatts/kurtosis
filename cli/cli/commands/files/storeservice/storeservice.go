package storeservice

import (
	"context"
	"fmt"
	"github.com/kurtosis-tech/kurtosis/api/golang/core/lib/enclaves"
	"github.com/kurtosis-tech/kurtosis/api/golang/core/lib/services"
	"github.com/kurtosis-tech/kurtosis/api/golang/engine/kurtosis_engine_rpc_api_bindings"
	"github.com/kurtosis-tech/kurtosis/api/golang/engine/lib/kurtosis_context"
	"github.com/kurtosis-tech/kurtosis/cli/cli/command_framework/highlevel/enclave_id_arg"
	"github.com/kurtosis-tech/kurtosis/cli/cli/command_framework/highlevel/engine_consuming_kurtosis_command"
	"github.com/kurtosis-tech/kurtosis/cli/cli/command_framework/lowlevel/args"
	"github.com/kurtosis-tech/kurtosis/cli/cli/command_framework/lowlevel/flags"
	"github.com/kurtosis-tech/kurtosis/cli/cli/command_str_consts"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface"
	"github.com/kurtosis-tech/stacktrace"
	"github.com/sirupsen/logrus"
)

const (
	enclaveIdentifierArgKey = "enclave-identifier"
	isEnclaveIdArgOptional  = false
	isEnclaveIdArgGreedy    = false

	serviceIdArgKey        = "service-id"
	absoluteFilepathArgKey = "filepath"

	nameFlagKey = "name"
	defaultName = ""

	kurtosisBackendCtxKey = "kurtosis-backend"
	engineClientCtxKey    = "engine-client"

	starlarkTemplate = `
CURRENT_TIME_STR = str(time.now().unix)
ARTIFACT_NAME = "cli-stored-artifact-" + CURRENT_TIME_STR
def run(plan, args):
	name = ARTIFACT_NAME
	if args.name != "":
		name = args.name
	plan.store_service_files(
		name = name,
		service_id = args.service_id,
		src = args.src
	)
`
)

var FilesStoreServiceCmd = &engine_consuming_kurtosis_command.EngineConsumingKurtosisCommand{
	CommandStr:       command_str_consts.FilesStoreServiceCmdStr,
	ShortDescription: "Store files from a service",
	LongDescription: fmt.Sprintf(
		"Instructs Kurtosis to copy a file or folder from the given absolute filepath in "+
			"the given service and store it in the enclave for later use (e.g. with '%v %v')",
		command_str_consts.ServiceCmdStr,
		command_str_consts.ServiceAddCmdStr,
	),
	KurtosisBackendContextKey: kurtosisBackendCtxKey,
	EngineClientContextKey:    engineClientCtxKey,
	Flags: []*flags.FlagConfig{
		{
			Key:     nameFlagKey,
			Usage:   "The name to be given to the produced of the artifact, auto generated if not passed",
			Type:    flags.FlagType_String,
			Default: defaultName,
		},
	},
	Args: []*args.ArgConfig{
		enclave_id_arg.NewEnclaveIdentifierArg(
			enclaveIdentifierArgKey,
			engineClientCtxKey,
			isEnclaveIdArgOptional,
			isEnclaveIdArgGreedy,
		),
		{
			Key: serviceIdArgKey,
		},
		{
			Key: absoluteFilepathArgKey,
		},
	},
	RunFunc: run,
}

func run(
	ctx context.Context,
	kurtosisBackend backend_interface.KurtosisBackend,
	engineClient kurtosis_engine_rpc_api_bindings.EngineServiceClient,
	flags *flags.ParsedFlags,
	args *args.ParsedArgs,
) error {
	enclaveIdentifier, err := args.GetNonGreedyArg(enclaveIdentifierArgKey)
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred getting the enclave ID using key '%v'", enclaveIdentifierArgKey)
	}

	serviceIdStr, err := args.GetNonGreedyArg(serviceIdArgKey)
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred getting the service ID value using key '%v'", serviceIdArgKey)
	}
	serviceId := services.ServiceID(serviceIdStr)

	filepath, err := args.GetNonGreedyArg(absoluteFilepathArgKey)
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred getting the absolute filepath value using key '%v'", absoluteFilepathArgKey)
	}

	artifactName, err := flags.GetString(nameFlagKey)
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred getting the name to be given to the produced artifact")
	}

	kurtosisCtx, err := kurtosis_context.NewKurtosisContextFromLocalEngine()
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred connecting to the local Kurtosis engine")
	}
	enclaveCtx, err := kurtosisCtx.GetEnclaveContext(ctx, enclaveIdentifier)
	if err != nil {
		return stacktrace.Propagate(err, "An error occurred getting the enclave context for enclave '%v'", enclaveIdentifier)
	}
	runResult, err := storeServiceFileStarlarkCommand(ctx, enclaveCtx, serviceId, filepath, enclaveIdentifier, artifactName)
	if err != nil {
		return stacktrace.Propagate(
			err,
			"An error occurred copying content from filepath '%v' in user service with ID '%v' to enclave '%v'",
			filepath,
			serviceId,
			enclaveIdentifier,
		)
	}
	logrus.Info(runResult.RunOutput)
	return nil
}

func storeServiceFileStarlarkCommand(ctx context.Context, enclaveCtx *enclaves.EnclaveContext, serviceId services.ServiceID, filePath string, enclaveIdentifier string, artifactName string) (*enclaves.StarlarkRunResult, error) {
	runResult, err := enclaveCtx.RunStarlarkScriptBlocking(ctx, starlarkTemplate, fmt.Sprintf(`{"service_id": "%s", "src": "%s", "name": "%s"}`, serviceId, filePath, artifactName), false)
	if err != nil {
		return nil, stacktrace.Propagate(
			err,
			"An unexpected error occurred running command for copying content from filepath '%v' in user service with ID '%v' to enclave '%v'. This is a bug in Kurtosis, please report.",
			filePath,
			serviceId,
			enclaveIdentifier)
	}
	if runResult.InterpretationError != nil {
		return nil, stacktrace.NewError(
			"An error occurred interpreting command for copying content from filepath '%v' in user service with ID '%v' to enclave '%v': %s\nThis is a bug in Kurtosis, please report.",
			filePath,
			serviceId,
			enclaveIdentifier,
			runResult.InterpretationError.GetErrorMessage(),
		)
	}
	if len(runResult.ValidationErrors) > 0 {
		return nil, stacktrace.NewError(
			"An error occurred validating command for copying content from filepath '%v' in user service with ID '%v' to enclave '%v': %v",
			filePath,
			serviceId,
			enclaveIdentifier,
			runResult.ValidationErrors,
		)
	}
	if runResult.ExecutionError != nil {
		return nil, stacktrace.NewError(
			"An error occurred executing command for copying content from filepath '%v' in user service with ID '%v' to enclave '%v': %s",
			filePath,
			serviceId,
			enclaveIdentifier,
			runResult.ExecutionError.GetErrorMessage(),
		)
	}
	return runResult, err
}
