/*
 * Copyright (c) 2021 - present Kurtosis Technologies Inc.
 * All Rights Reserved.
 */

package commands

import (
	"encoding/json"
	"github.com/Masterminds/semver/v3"
	"github.com/kurtosis-tech/kurtosis-cli/cli/commands/clean"
	"github.com/kurtosis-tech/kurtosis-cli/cli/commands/enclave"
	"github.com/kurtosis-tech/kurtosis-cli/cli/commands/engine"
	"github.com/kurtosis-tech/kurtosis-cli/cli/commands/module"
	"github.com/kurtosis-tech/kurtosis-cli/cli/commands/repl"
	"github.com/kurtosis-tech/kurtosis-cli/cli/commands/sandbox"
	"github.com/kurtosis-tech/kurtosis-cli/cli/commands/service"
	"github.com/kurtosis-tech/kurtosis-cli/cli/commands/test"
	"github.com/kurtosis-tech/kurtosis-cli/cli/commands/version"
	"github.com/kurtosis-tech/kurtosis-cli/cli/helpers/logrus_log_levels"
	"github.com/kurtosis-tech/kurtosis-cli/cli/kurtosis_cli_version"
	"github.com/palantir/stacktrace"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
)

const (
	logLevelStrArg = "cli-log-level"

	latestReleaseOnGitHubURL   = "https://api.github.com/repos/kurtosis-tech/kurtosis-cli-release-artifacts/releases/latest"
	acceptHttpHeaderKey        = "Accept"
	acceptHttpHeaderValue      = "application/json"
	contentTypeHttpHeaderKey   = "Content-Type"
	contentTypeHttpHeaderValue = "application/json"
	userAgentHttpHeaderKey     = "User-Agent"
	userAgentHttpHeaderValue   = "kurtosis-tech"

	upgradeCLIInstructionsDocsPageURL = "https://docs.kurtosistech.com/installation.html#upgrading-kurtosis-cli"
)

type GitHubReleaseReponse struct {
	TagName string `json:"tag_name"`
}

var logLevelStr string
var defaultLogLevelStr = logrus.InfoLevel.String()

var RootCmd = &cobra.Command{
	// Leaving out the "use" will auto-use os.Args[0]
	Use:                        "",
	Short: "A CLI for interacting with the Kurtosis engine",

	// Cobra will print usage whenever _any_ error occurs, including ones we throw in Kurtosis
	// This doesn't make sense in 99% of the cases, so just turn them off entirely
	SilenceUsage: true,
	PersistentPreRunE: globalSetup,
}

func init() {
	RootCmd.PersistentFlags().StringVar(
		&logLevelStr,
		logLevelStrArg,
		defaultLogLevelStr,
		"Sets the level that the CLI will log at (" + strings.Join(logrus_log_levels.GetAcceptableLogLevelStrs(), "|") + ")",
	)

	RootCmd.AddCommand(sandbox.SandboxCmd)
	RootCmd.AddCommand(test.TestCmd)
	RootCmd.AddCommand(enclave.EnclaveCmd)
	RootCmd.AddCommand(service.ServiceCmd)
	RootCmd.AddCommand(module.ModuleCmd)
	RootCmd.AddCommand(repl.REPLCmd)
	RootCmd.AddCommand(engine.EngineCmd)
	RootCmd.AddCommand(version.VersionCmd)
	RootCmd.AddCommand(clean.CleanCmd)
}

// ====================================================================================================
//                                       Private Helper Functions
// ====================================================================================================
func globalSetup(cmd *cobra.Command, args []string) error {
	logLevel, err := logrus.ParseLevel(logLevelStr)
	if err != nil {
		return stacktrace.Propagate(err, "Could not parse log level string '%v'", logLevelStr)
	}
	logrus.SetOutput(cmd.OutOrStdout())
	logrus.SetLevel(logLevel)

	checkCLIVersion()

	return nil
}

func checkCLIVersion() {
	isLatestVersion, latestVersion, err := isLatestCLIVersion()
	if err != nil {
		logrus.Warning("An error occurred trying to check if you are running the latest Kurtosis CLI version.")
		logrus.Debugf("Checking latest version error: %v", err)
		logrus.Warningf("Your current version is '%v'", kurtosis_cli_version.KurtosisCLIVersion)
		logrus.Warningf("You can manually upgrade the CLI tool following these instructions: %v", upgradeCLIInstructionsDocsPageURL)
		return
	}
	if !isLatestVersion {
		logrus.Warningf("You are running an old version of the Kurtosis CLI; we suggest you to update it to the latest version, '%v'", latestVersion)
		logrus.Warningf("You can manually upgrade the CLI tool following these instructions: %v", upgradeCLIInstructionsDocsPageURL)
	}
	return
}

func isLatestCLIVersion() (bool, string, error) {
	ownVersionStr := kurtosis_cli_version.KurtosisCLIVersion
	latestVersionStr, err := getLatestCLIReleaseVersionFromGitHub()
	if err != nil {
		return false, "", stacktrace.Propagate(err, "An error occurred getting the latest release version number from the GitHub public API")
	}

	if ownVersionStr == latestVersionStr {
		return true, latestVersionStr, nil
	}

	ownSemver, err := semver.StrictNewVersion(ownVersionStr)
	if err != nil {
		return false, "", stacktrace.Propagate(err, "An error occurred parsing own version string '%v' to sem version", ownVersionStr)
	}

	latestSemver, err := semver.StrictNewVersion(latestVersionStr)
	if err != nil {
		return false, "", stacktrace.Propagate(err, "An error occurred parsing latest version string '%v' to sem version", latestVersionStr)
	}

	compareResult := ownSemver.Compare(latestSemver)

	//compareResult = 1  means that the own version is newer than the latest version, (e.g.: during a new release)
	if compareResult >= 0 {
		return true, latestVersionStr, nil
	}

	return false, latestVersionStr, nil
}

func getLatestCLIReleaseVersionFromGitHub() (string, error) {
	var (
		client         = &http.Client{}
		requestMethod  = "GET"
		requestBody    io.Reader
		responseObject GitHubReleaseReponse
	)

	request, err := http.NewRequest(requestMethod, latestReleaseOnGitHubURL, requestBody)
	if err != nil {
		return "", stacktrace.Propagate(err, "An error occurred creating new HTTP GET request to URL '%v' ", latestReleaseOnGitHubURL)
	}

	request.Header.Add(acceptHttpHeaderKey, acceptHttpHeaderValue)
	request.Header.Add(contentTypeHttpHeaderKey, contentTypeHttpHeaderValue)
	request.Header.Add(userAgentHttpHeaderKey, userAgentHttpHeaderValue)

	response, err := client.Do(request)
	if err != nil {
		return "", stacktrace.Propagate(err, "An error occurred executing HTTP GET request to URL '%v' ", latestReleaseOnGitHubURL)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			logrus.Warnf("We tried to close the response body, but doing so threw an error:\n%v", err)
		}
	}()

	bodyBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", stacktrace.Propagate(err, "An error occurred reading the HTTP response body")
	}

	if err := json.Unmarshal(bodyBytes, &responseObject); err != nil {
		return "", stacktrace.Propagate(err, "An error occurred deserializing the latest release body response")
	}

	latestVersion := responseObject.TagName
	if latestVersion == "" {
		return "", stacktrace.Propagate(err, "The latest release version got from GitHub releases is empty")
	}

	return latestVersion, nil
}
