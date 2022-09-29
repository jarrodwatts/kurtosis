#!/usr/bin/env bash
# 2021-07-08 WATERMARK, DO NOT REMOVE - This script was generated from the Kurtosis Bash script template

set -euo pipefail   # Bash "strict mode"
script_dirpath="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root_dirpath="$(dirname "${script_dirpath}")"
run_pre_release_scripts_script_path="${script_dirpath}/run-tests-locally-against-latest-engine-and-core.sh"
cli_launch_path="${root_dirpath}/cli/cli/scripts/launch-cli.sh"
internal_test_suite_build_script_path="${root_dirpath}/internal_testsuites/scripts/build.sh"


# ==================================================================================================
#                                             Constants
# ==================================================================================================

BUILD_SCRIPT_RELATIVE_FILEPATHS=(
  "core/server/scripts/build.sh"
  "core/files_artifacts_expander/scripts/build.sh"
  "engine/server/scripts/build.sh"
  "cli/scripts/build.sh"
)


TESTSUITE_CLUSTER_BACKEND_DOCKER="docker"
TESTSUITE_CLUSTER_BACKEND_MINIKUBE="minikube"

# By default, run testsuite against docker
DEFAULT_TESTSUITE_CLUSTER_BACKEND="${TESTSUITE_CLUSTER_BACKEND_DOCKER}"


# ==================================================================================================
#                                       Arg Parsing & Validation
# ==================================================================================================
show_helptext_and_exit() {
    echo "Usage: $(basename "${0}") cli_cluster_backend_arg"
    echo ""
    echo "  cli_cluster_backend_arg   Optional argument describing the cluster backend tests are running against. Must be one of 'docker', 'minikube' (default: ${DEFAULT_TESTSUITE_CLUSTER_BACKEND})"
    echo ""
    exit 1  # Exit with an error so that if this is accidentally called by CI, the script will fail
}

testsuite_cluster_backend_arg="${1:-${DEFAULT_TESTSUITE_CLUSTER_BACKEND}}"
if [ "${testsuite_cluster_backend_arg}" != "${TESTSUITE_CLUSTER_BACKEND_DOCKER}" ] &&
   [ "${testsuite_cluster_backend_arg}" != "${TESTSUITE_CLUSTER_BACKEND_MINIKUBE}" ]; then
    echo "Error: unknown cluster provided to run tests against. Must be one of 'docker', 'minikube'"
    show_helptext_and_exit
fi

# ==================================================================================================
#                                             Main Logic
# ==================================================================================================

if !bash "${run_pre_release_scripts_script_path}"; then
  echo "Error: Error running pre release scripts '${run_pre_release_scripts_script_path}' failed" >&2
  exit 1
fi

# if the test suite is k8s we build & run images in k8s
# we start minikube if it isn't running
# we set the docker to be the one inside minikube
if [ "${testsuite_cluster_backend_arg}" == "${TESTSUITE_CLUSTER_BACKEND_MINIKUBE}" ]; then
    if ! minikube status; then
      if ! minikube start; then
          echo "Error starting minikube" >&2
          exit 1
      fi
    fi
    if ! eval $(minikube docker-env); then
        echo "Error changing docker environment to minikube" >&2
        exit 1
    fi
fi

for build_script_rel_filepath in "${BUILD_SCRIPT_RELATIVE_FILEPATHS[@]}"; do
    build_script_abs_filepath="${root_dirpath}/${build_script_rel_filepath}"
    if ! bash "${build_script_abs_filepath}"; then
        echo "Error: Build script '${build_script_abs_filepath}' failed" >&2
        exit 1
    fi
done

# stop existing engine
if ! bash "${cli_launch_path}" engine stop; then
    echo "Error: Stopping the engine failed" >&2
    exit 1
fi