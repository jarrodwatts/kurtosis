package engine_functions

import (
	"context"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_impls/docker/docker_manager"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_impls/docker/docker_operation_parallelizer"
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_interface/objects/engine"
	"github.com/kurtosis-tech/stacktrace"
)

func DestroyEngines(
	ctx context.Context,
	filters *engine.EngineFilters,
	dockerManager *docker_manager.DockerManager,
) (
	resultSuccessfulEngineGuids map[engine.EngineGUID]bool,
	resultErroredEngineGuids map[engine.EngineGUID]error,
	resultErr error,
) {
	matchingEnginesByContainerId, err := getMatchingEngines(ctx, filters, dockerManager)
	if err != nil {
		return nil, nil, stacktrace.Propagate(err, "An error occurred getting engines matching the following filters: %+v", filters)
	}

	logsComponentsContainersByEngineContainerId, err := getLogsComponentsContainerIdsByEngineContainerIds(ctx, matchingEnginesByContainerId, dockerManager)
	if err != nil {
		return nil, nil, stacktrace.Propagate(err, "An error occurred getting logs componentes containers by engine container IDs '%+v'", matchingEnginesByContainerId)
	}

	// TODO PLEAAASE GO GENERICS... but we can't use 1.18 yet because it'll break all Kurtosis clients :(
	matchingUncastedEnginesByContainerId := map[string]interface{}{}
	for containerId, engineObj := range matchingEnginesByContainerId {
		matchingUncastedEnginesByContainerId[containerId] = interface{}(engineObj)
	}

	var removeEngineOperation docker_operation_parallelizer.DockerOperation = func(
		ctx context.Context,
		dockerManager *docker_manager.DockerManager,
		dockerObjectId string,
	) error {
		engineContainerId := dockerObjectId
		if err := dockerManager.RemoveContainer(ctx, engineContainerId); err != nil {
			return stacktrace.Propagate(err, "An error occurred removing engine container with ID '%v'", engineContainerId)
		}

		logsComponentsToRemoveContainerIDs, found := logsComponentsContainersByEngineContainerId[engineContainerId]
		if !found {
			return nil
		}

		if logsComponentsToRemoveContainerIDs.logsDatabaseContainerId != "" {
			if err := dockerManager.RemoveContainer(ctx, logsComponentsToRemoveContainerIDs.logsDatabaseContainerId); err != nil {
				return stacktrace.Propagate(err, "An error occurred removing logs database container with ID '%v'", logsComponentsToRemoveContainerIDs.logsDatabaseContainerId)
			}
		}

		if logsComponentsToRemoveContainerIDs.logsCollectorContainerId != "" {
			if err := dockerManager.RemoveContainer(ctx, logsComponentsToRemoveContainerIDs.logsCollectorContainerId); err != nil {
				return stacktrace.Propagate(err, "An error occurred removing logs collector container with ID '%v'", logsComponentsToRemoveContainerIDs.logsCollectorContainerId)
			}
		}

		return nil
	}

	successfulEngineGuidStrs, erroredEngineGuidStrs, err := docker_operation_parallelizer.RunDockerOperationInParallelForKurtosisObjects(
		ctx,
		matchingUncastedEnginesByContainerId,
		dockerManager,
		extractEngineGuidFromUncastedEngineObj,
		removeEngineOperation,
	)
	if err != nil {
		return nil, nil, stacktrace.Propagate(err, "An error occurred removing engine containers matching filters '%+v'", filters)
	}

	successfulGuids := map[engine.EngineGUID]bool{}
	for guidStr := range successfulEngineGuidStrs {
		successfulGuids[engine.EngineGUID(guidStr)] = true
	}

	erroredGuids := map[engine.EngineGUID]error{}
	for guidStr, err := range erroredEngineGuidStrs {
		erroredGuids[engine.EngineGUID(guidStr)] = stacktrace.Propagate(
			err,
		"An error occurred destroying engine '%v'",
			guidStr,
	)
	}

	//TODO destroy centralized logs components (logs database and logs collector) containers also

	return successfulGuids, erroredGuids, nil
}

