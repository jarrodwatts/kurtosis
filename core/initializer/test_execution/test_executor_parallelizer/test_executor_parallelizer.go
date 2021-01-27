/*
 * Copyright (c) 2020 - present Kurtosis Technologies LLC.
 * All Rights Reserved.
 */

package test_executor_parallelizer

import (
	"context"
	"fmt"
	"github.com/docker/docker/client"
	"github.com/google/uuid"
	"github.com/kurtosis-tech/kurtosis/initializer/test_execution/test_executor"
	"github.com/kurtosis-tech/kurtosis/initializer/test_suite_launcher"
	"github.com/palantir/stacktrace"
	"github.com/sirupsen/logrus"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
)

/*
Runs the given tests in parallel, printing:
1) the output of tests as they finish
2) a summary of all tests once all tests have finished

Args:
	executionId: The UUID uniquely identifying this execution of the tests
	dockerClient: The handle to manipulating the Docker environment
	parallelism: The number of tests to run concurrently
	allTestParams: A mapping of test_name -> parameters for running the test
	testSuiteImageName: The name of the Docker image that will be used to run the test controller
	testSuiteLogLevel: A string, meaningful to the test controller, that represents the user's desired log level
	customTestSuiteEnvVars: A custom user-defined map from <env variable name> -> <env variable value> that will be
		passed via Docker environment variables to the test controller

Returns:
	True if all tests passed, false otherwise
 */
func RunInParallelAndPrintResults(
		executionId uuid.UUID,
		dockerClient *client.Client,
		parallelism uint,
		allTestParams map[string]ParallelTestParams,
		testsuiteLauncher *test_suite_launcher.TestsuiteContainerLauncher) bool {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	// Set up listener for exit signals so we handle it nicely
	sigs := make(chan os.Signal, 1)
	defer close(sigs)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	// Asynchronously handle graceful exit signals by cancelling context.
	go func() {
		sig, ok := <-sigs
		// signal channel was closed with no syscall signal
		if !ok { return }
		fmt.Printf("\nReceived signal: %v. Cleaning up tests and exiting gracefully...\n", sig)
		cancelFunc()
		// TODO send message to all the parallel threads that they should tear down immediately
	}()
	// These need to be buffered else sending to the channel will be blocking
	testParamsChan := make(chan ParallelTestParams, len(allTestParams))

	logrus.Info("Loading test params into work queue...")
	for _, testParams := range allTestParams {
		testParamsChan <- testParams
	}
	close(testParamsChan) // We close the channel so that when all params are consumed, the worker threads won't block on waiting for more params
	logrus.Info("All test params loaded into work queue")

	outputManager := newParallelTestOutputManager()

	logrus.Infof("Launching %v tests with parallelism %v...", len(allTestParams), parallelism)

	disableSystemLogAndRunTestThreads(
		executionId,
		ctx,
		outputManager,
		testParamsChan,
		parallelism,
		dockerClient,
		testsuiteLauncher)

	logrus.Info("All tests exited")

	outputManager.printSummary()
	return outputManager.getAllTestsPassed()
}


func disableSystemLogAndRunTestThreads(
		executionId uuid.UUID,
		parentContext context.Context,
		outputManager *ParallelTestOutputManager,
		testParamsChan chan ParallelTestParams,
		parallelism uint,
		dockerClient *client.Client,
		testsuiteLauncher *test_suite_launcher.TestsuiteContainerLauncher) {
		/*
		    Because each test needs to have its logs written to an independent file to avoid getting logs all mixed up, we need to make
    sure that all code below this point uses the per-test logger rather than the systemwide logger. However, it's very difficult for
    a coder to remember to use 'log.Info' when they're used to doing 'logrus.Info'. To enforce this, we capture any systemwide logger usages
	during this function so we can show them later.
	*/
	outputManager.startInterceptingStdLogger()
	defer outputManager.stopInterceptingStdLogger()

	var waitGroup sync.WaitGroup
	for i := uint(0); i < parallelism; i++ {
		waitGroup.Add(1)
		go runTestWorkerGoroutine(
			executionId,
			parentContext,
			&waitGroup,
			testParamsChan,
			outputManager,
			dockerClient,
			testsuiteLauncher)
	}
	waitGroup.Wait()
}

/*
A function, designed to be run inside a worker thread, that will pull test params from the given test params channel, execute the test, and
push the result to the test results channel
 */
func runTestWorkerGoroutine(
			executionId uuid.UUID,
			parentContext context.Context,
			waitGroup *sync.WaitGroup,
			testParamsChan chan ParallelTestParams,
			outputManager *ParallelTestOutputManager,
			dockerClient *client.Client,
			testsuiteLauncher *test_suite_launcher.TestsuiteContainerLauncher) {
	// IMPORTANT: make sure that we mark a thread as done!
	defer waitGroup.Done()

	for testParams := range testParamsChan {
		testName := testParams.TestName

		tempFilename := fmt.Sprintf("%v-%v", executionId, testName)
		writingTempFp, err := ioutil.TempFile("", tempFilename)
		if err != nil {
			emptyOutputReader := &strings.Reader{}
			executionErr := stacktrace.Propagate(err, "An error occurred creating temporary file to contain logs of test %v", testName)
			outputManager.logTestOutput(testName, executionErr, false, emptyOutputReader)
			continue
		}
		defer writingTempFp.Close()

		// Create a separate logger just for this test that writes to the test execution logfile
		log := logrus.New()
		log.SetLevel(logrus.GetLevel())
		log.SetOutput(writingTempFp)
		log.SetFormatter(logrus.StandardLogger().Formatter)

		testsuiteDebuggerHostPortBinding := testParams.DebuggerHostPortBinding

		outputManager.logTestLaunch(testName, testsuiteDebuggerHostPortBinding)
		passed, executionErr := test_executor.RunTest(
			executionId,
			parentContext,
			log,
			dockerClient,
			testParams.SubnetMask,
			testsuiteLauncher,
			testsuiteDebuggerHostPortBinding,
			testName,
			testParams.TestMetadata)
		writingTempFp.Close() // Close to flush out anything remaining in the buffer

		// Create a new FP to read the logfile from the start
		var testOutputReader io.Reader
		readingTempFp, err := os.Open(writingTempFp.Name())
		if err != nil {
			errorMsg := fmt.Sprintf("An error occurred opening the test's logfile for reading; logs for this test are unavailable:\n%s", err)
			testOutputReader = strings.NewReader(errorMsg)
		} else {
			defer readingTempFp.Close()
			testOutputReader = readingTempFp
		}
		outputManager.logTestOutput(testName, executionErr, passed, testOutputReader)
	}
}
