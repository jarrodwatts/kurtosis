//go:build !minikube
// +build !minikube

// We don't run this test in Kubernetes because, as of 2022-10-28, the centralized logs feature is not implemented in Kubernetes yet
//TODO remove this comments after Kubernetes implementation

package search_logs_test

import (
	"context"
	"github.com/kurtosis-tech/kurtosis-cli/golang_internal_testsuite/test_helpers"
	"github.com/kurtosis-tech/kurtosis/api/golang/core/lib/services"
	"github.com/kurtosis-tech/kurtosis/api/golang/engine/lib/kurtosis_context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

const (
	testName              = "search-logs"
	isPartitioningEnabled = false

	exampleServiceIdPrefix = "search-logs-"

	shouldFollowLogs    = true
	shouldNotFollowLogs = false

	service1ServiceID services.ServiceID = exampleServiceIdPrefix + "service-1"

	firstFilterText     = "The data have being loaded"
	secondFilterText    = "Starting feature"
	thirdFilterText     = "network"
	matchRegexFilterStr = "Starting.*logs'"

	testTimeOut = 180 * time.Second

	logLine1 = "Starting feature 'centralized logs'"
	logLine2 = "Starting feature 'network partitioning'"
	logLine3 = "Starting feature 'network soft partitioning'"
	logLine4 = "The data have being loaded"
)

var (
	expectedNonExistenceServiceGuids = map[services.ServiceGUID]bool{}

	service1LogLines = []string{
		logLine1,
		logLine2,
		logLine3,
		logLine4,
	}

	logLinesByService = map[services.ServiceID][]string{
		service1ServiceID: service1LogLines,
	}

	doesContainTextFilterForFirstRequest  = kurtosis_context.NewDoesContainTextLogLineFilter(firstFilterText)
	doesContainTextFilterForSecondRequest = kurtosis_context.NewDoesContainTextLogLineFilter(secondFilterText)
	doesNotContainTextFilter              = kurtosis_context.NewDoesNotContainTextLogLineFilter(thirdFilterText)
	doesContainMatchRegexFilter           = kurtosis_context.NewDoesContainMatchRegexLogLineFilter(matchRegexFilterStr)
	doesNotContainMatchRegexFilter        = kurtosis_context.NewDoesNotContainMatchRegexLogLineFilter(matchRegexFilterStr)

	filtersByRequest = []kurtosis_context.LogLineFilter{
		*doesContainTextFilterForFirstRequest,
		*doesContainTextFilterForSecondRequest,
		*doesNotContainTextFilter,
		*doesContainMatchRegexFilter,
		*doesNotContainMatchRegexFilter,
	}

	expectedLogLinesByRequest = [][]string{
		{
			logLine4,
		},
		{
			logLine1,
			logLine2,
			logLine3,
		},
		{
			logLine1,
			logLine4,
		},
		{
			logLine1,
		},
		{
			logLine2,
			logLine3,
			logLine4,
		},
	}

	shouldFollowLogsValueByRequest = []bool{
		shouldFollowLogs,
		shouldNotFollowLogs,
		shouldNotFollowLogs,
		shouldNotFollowLogs,
		shouldNotFollowLogs,
	}
)

func TestSearchLogs(t *testing.T) {
	ctx := context.Background()

	// ------------------------------------- ENGINE SETUP ----------------------------------------------
	enclaveCtx, stopEnclaveFunc, _, err := test_helpers.CreateEnclave(t, ctx, testName, isPartitioningEnabled)
	require.NoError(t, err, "An error occurred creating an enclave")
	defer stopEnclaveFunc()

	// ------------------------------------- TEST SETUP ----------------------------------------------
	kurtosisCtx, err := kurtosis_context.NewKurtosisContextFromLocalEngine()
	require.NoError(t, err)

	serviceList, err := test_helpers.AddServicesWithLogLines(ctx, enclaveCtx, logLinesByService)
	require.NoError(t, err, "An error occurred adding services with log lines '%+v'", logLinesByService)
	require.Equal(t, len(logLinesByService), len(serviceList))

	// ------------------------------------- TEST RUN -------------------------------------------------
	enclaveUuid := enclaveCtx.GetEnclaveUuid()

	userServiceGuids := map[services.ServiceGUID]bool{}
	for _, serviceCtx := range serviceList {
		serviceGuid := serviceCtx.GetServiceGUID()
		userServiceGuids[serviceGuid] = true
	}

	expectedLogLinesByService := map[services.ServiceGUID][]string{}

	for requestIndex, filter := range filtersByRequest {

		for serviceGuid := range userServiceGuids {
			expectedLogLinesByService[serviceGuid] = expectedLogLinesByRequest[requestIndex]
		}

		shouldFollowLogsOption := shouldFollowLogsValueByRequest[requestIndex]

		testEvaluationErr, receivedLogLinesByService, receivedNotFoundServiceGuids := test_helpers.GetLogsResponse(
			t,
			ctx,
			testTimeOut,
			kurtosisCtx,
			string(enclaveUuid),
			userServiceGuids,
			expectedLogLinesByService,
			shouldFollowLogsOption,
			&filter,
		)

		require.NoError(t, testEvaluationErr)
		for serviceGuid := range userServiceGuids {
			require.Equal(t, expectedLogLinesByRequest[requestIndex], receivedLogLinesByService[serviceGuid])
		}
		require.Equal(t, expectedNonExistenceServiceGuids, receivedNotFoundServiceGuids)
	}
}
