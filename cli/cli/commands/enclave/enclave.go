/*
 * Copyright (c) 2021 - present Kurtosis Technologies Inc.
 * All Rights Reserved.
 */

package enclave

import (
	"github.com/kurtosis-tech/kurtosis-cli/cli/command_str_consts"
	"github.com/kurtosis-tech/kurtosis-cli/cli/commands/enclave/dump"
	"github.com/kurtosis-tech/kurtosis-cli/cli/commands/enclave/inspect"
	"github.com/kurtosis-tech/kurtosis-cli/cli/commands/enclave/ls"
	"github.com/kurtosis-tech/kurtosis-cli/cli/commands/enclave/new"
	"github.com/kurtosis-tech/kurtosis-cli/cli/commands/enclave/rm"
	"github.com/kurtosis-tech/kurtosis-cli/cli/commands/enclave/stop"
	"github.com/spf13/cobra"
)

var EnclaveCmd = &cobra.Command{
	Use:   command_str_consts.EnclaveCmdStr,
	Short: "Manage enclaves",
	RunE:  nil,
}

func init() {
	EnclaveCmd.AddCommand(ls.LsCmd)
	EnclaveCmd.AddCommand(inspect.EnclaveInspectCmd.MustGetCobraCommand())
	EnclaveCmd.AddCommand(new.NewCmd)
	EnclaveCmd.AddCommand(stop.StopCmd)
	EnclaveCmd.AddCommand(rm.EnclaveRmCmd.MustGetCobraCommand())
	EnclaveCmd.AddCommand(dump.EnclaveDumpCmd)
}
