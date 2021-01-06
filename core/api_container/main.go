/*
 * Copyright (c) 2020 - present Kurtosis Technologies LLC.
 * All Rights Reserved.
 */

package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/docker/docker/client"
	"github.com/gorilla/rpc/v2"
	"github.com/gorilla/rpc/v2/json2"
	"github.com/kurtosis-tech/kurtosis/api_container/api"
	api_container_docker_consts2 "github.com/kurtosis-tech/kurtosis/api_container/api_container_docker_consts"
	"github.com/kurtosis-tech/kurtosis/api_container/execution/exit_codes"
	"github.com/kurtosis-tech/kurtosis/api_container/execution/test_execution_status"
	"github.com/kurtosis-tech/kurtosis/api_container/service_network"
	"github.com/kurtosis-tech/kurtosis/api_container/service_network/user_service_launcher"
	"github.com/kurtosis-tech/kurtosis/commons"
	"github.com/kurtosis-tech/kurtosis/commons/logrus_log_levels"
	"github.com/palantir/stacktrace"
	"github.com/sirupsen/logrus"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const (

	// If no test suite registers a test execution in this time, the API container will shut itself down of its own accord
	idleShutdownTimeout = 15 * time.Second

	// When shutting down the service network, the maximum amount of time we'll give a container to stop gracefully
	//  before hard-killing it
	containerStopTimeout = 10 * time.Second
)

func main() {
	// NOTE: we'll want to chnage the ForceColors to false if we ever want structured logging
	logrus.SetFormatter(&logrus.TextFormatter{
		ForceColors:   true,
		FullTimestamp: true,
	})


	networkIdArg := flag.String(
		"network-id",
		"",
		"ID of the Docker network that the API container is running in, and in which all services should be started",
	)

	subnetMaskArg := flag.String(
		"subnet-mask",
		"",
		"Subnet mask of the Docker network that the API container is running in",
	)

	gatewayIpArg := flag.String(
		"gateway-ip",
		"",
		"IP address of the gateway address on the Docker network that the test controller is running in",
	)

	testVolumeNameArg := flag.String(
		"test-volume",
		"",
		"Name of the test volume that should be mounted on every new service",
	)

	testSuiteContainerIdArg := flag.String(
		"test-suite-container-id",
		"",
		"ID of the Docker container running the test suite",
	)

	// It seems weird that we require this given that the test suite container doesn't run a server, but it's only so
	//  that our free IP address tracker knows not to dole out the test suite container's IP address
	testSuiteContainerIpAddrArg := flag.String(
		"test-suite-container-ip",
		"",
		"IP address of the Docker container running the test suite container",
	)

	apiContainerIpAddrArg := flag.String(
		"api-container-ip",
		"",
		"IP address of the Docker container running the API container",
	)

	logLevelArg := flag.String(
		"log-level",
		"info",
		fmt.Sprintf("Log level to use for the API container (%v)", logrus_log_levels.AcceptableLogLevels),
	)

	isPartitioningEnabledArg := flag.Bool(
		"is-partitioning-enabled",
		false,
		"True if partitioning is enabled, signifying that the API container should run the extra machinery needed to support partitioning")

	flag.Parse()

	logLevel, err := logrus.ParseLevel(*logLevelArg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "An error occurred parsing the log level string: %v\n", err)
		os.Exit(exit_codes.StartupErrorExitCode)
	}
	logrus.SetLevel(logLevel)

	dockerManager, err := createDockerManager()
	if err != nil {
		logrus.Error("An error occurred creating the Docker manager:")
		fmt.Fprint(logrus.StandardLogger().Out, err)
		os.Exit(exit_codes.StartupErrorExitCode)
	}

	serviceNetwork, err := createServiceNetwork(
		dockerManager,
		*networkIdArg,
		*subnetMaskArg,
		*gatewayIpArg,
		*apiContainerIpAddrArg,
		*testSuiteContainerIpAddrArg,
		*testVolumeNameArg,
		*isPartitioningEnabledArg)
	if err != nil {
		logrus.Error("An error occurred creating the service network:")
		fmt.Fprint(logrus.StandardLogger().Out, err)
		os.Exit(exit_codes.StartupErrorExitCode)
	}

	// A value on this channel indicates a change in the test execution status
	testExecutionStatusChan := make(chan test_execution_status.TestExecutionStatus, 1)

	server, err := createServer(
		dockerManager,
		testExecutionStatusChan,
		*testSuiteContainerIdArg,
		serviceNetwork)
	if err != nil {
		logrus.Error("Failed to create a server with the following error:")
		fmt.Fprint(logrus.StandardLogger().Out, err)
		os.Exit(exit_codes.StartupErrorExitCode)
	}

	go func(){
		server.ListenAndServe()
	}()

	// Docker will send SIGTERM to end the process, and we need to catch it to stop gracefully
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	logrus.Info("Waiting for stop signal or test completion...")
	exitCode := waitUntilExitCondition(testExecutionStatusChan, signalChan)

	// NOTE: Might need to kick off a timeout thread to separately close the context if it's taking too long or if
	//  the server hangs forever trying to shutdown
	logrus.Info("Shutting down JSON RPC server...")
	if err := server.Shutdown(context.Background()); err != nil {
		logrus.Error("An error occurred shutting down the JSON RPC server:")
		fmt.Fprint(logrus.StandardLogger().Out, err)
		exitCode = exit_codes.ShutdownErrorExitCode
	} else {
		logrus.Info("JSON RPC server shut down successfully")
	}

	// NOTE: Might need to kick off a timeout thread to separately close the context if it's taking too long or if
	//  the service network hangs forever trying to shutdown
	logrus.Info("Destroying service network...")
	if err := serviceNetwork.Destroy(context.Background(), containerStopTimeout); err != nil {
		logrus.Error("An error occurred destroying the service network:")
		fmt.Fprint(logrus.StandardLogger().Out, err)
		exitCode = exit_codes.ShutdownErrorExitCode
	} else {
		logrus.Info("Service network destroyed successfully")
	}

	os.Exit(exitCode)
}

func createDockerManager() (*commons.DockerManager, error) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, stacktrace.Propagate(err, "Could not initialize a Docker client from the environment")
	}

	dockerManager, err := commons.NewDockerManager(logrus.StandardLogger(), dockerClient)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred creating the Docker manager")
	}

	return dockerManager, nil
}

func createServiceNetwork(
		dockerManager *commons.DockerManager,
		dockerNetworkId string,
		networkSubnetMask string,
		gatewayIp string,
		apiContainerIp string,
		testSuiteContainerIp string,
		testVolumeName string,
		isPartitioningEnabled bool) (*service_network.ServiceNetwork, error) {

	freeIpAddrTracker, err := commons.NewFreeIpAddrTracker(
		logrus.StandardLogger(),
		networkSubnetMask,
		map[string]bool{
			gatewayIp:      true,
			apiContainerIp: true,
			testSuiteContainerIp: true,
		})
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred creating the free IP address tracker")
	}

	userServiceLauncher := user_service_launcher.NewUserServiceLauncher(
		dockerManager,
		freeIpAddrTracker,
		dockerNetworkId,
		testVolumeName)

	serviceNetwork := service_network.NewServiceNetwork(
		isPartitioningEnabled,
		dockerNetworkId,
		freeIpAddrTracker,
		dockerManager,
		userServiceLauncher)
	return serviceNetwork, nil
}

func createServer(
		dockerManager *commons.DockerManager,
		testExecutionStatusChan chan test_execution_status.TestExecutionStatus,
		testSuiteContainerId string,
		serviceNetwork *service_network.ServiceNetwork) (*http.Server, error) {
	kurtosisService := api.NewKurtosisService(
		testSuiteContainerId,
		testExecutionStatusChan,
		dockerManager,
		serviceNetwork)

	logrus.Info("Launching server...")
	httpHandler := rpc.NewServer()
	jsonCodec := json2.NewCodec()
	httpHandler.RegisterCodec(jsonCodec, "application/json")
	httpHandler.RegisterService(kurtosisService, "")
	server := &http.Server{
		Addr:    fmt.Sprintf(":%v", api_container_docker_consts2.ContainerPort),
		Handler: httpHandler,
	}

	return server, nil
}

func waitUntilExitCondition(testExecutionStatusChan chan test_execution_status.TestExecutionStatus, signalChan chan os.Signal) int {
	// If no test suite registers within our idle timeout, shut the container down
	select {
	case receivedStatus := <-testExecutionStatusChan:
		if receivedStatus != test_execution_status.Running {
			logrus.Errorf(
				"Received out-of-order test execution status update; got %v when we were expecting %v",
				receivedStatus,
				test_execution_status.Running)
			return exit_codes.OutOfOrderTestStatusExitCode
		}
	case <-time.After(idleShutdownTimeout):
		logrus.Errorf("No test suite registered itself after waiting %v; this likely means the test suite had a fatal error", idleShutdownTimeout)
		return exit_codes.NoTestSuiteRegisteredExitCode
	case signal := <-signalChan:
		logrus.Infof("Received signal %v while awaiting test registration; server will shut down", signal)
		return exit_codes.ReceivedTermSignalExitCode
	}
	logrus.Info("Received notification that a test was registered; proceeding to await test completion...")

	select {
	case receivedStatus := <-testExecutionStatusChan:
		if !(receivedStatus == test_execution_status.HitTimeout || receivedStatus == test_execution_status.CompletedBeforeTimeout) {
			logrus.Errorf(
				"Received out-of-order test execution status update; got %v when we were expecting %v or %v",
				receivedStatus,
				test_execution_status.HitTimeout,
				test_execution_status.CompletedBeforeTimeout)
			return exit_codes.OutOfOrderTestStatusExitCode
		}
		if receivedStatus == test_execution_status.HitTimeout {
			logrus.Error("Test execution hit timeout before test completion")
			return exit_codes.TestHitTimeoutExitCode
		}
	case signal := <-signalChan:
		logrus.Infof("Received signal %v while awaiting test completion; server will shut down", signal)
		return exit_codes.ReceivedTermSignalExitCode
	}

	logrus.Info("Test completed before reaching the timeout")
	return exit_codes.TestCompletedInTimeoutExitCode
}
