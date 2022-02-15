package kurtosis_command

import (
	"github.com/kurtosis-tech/kurtosis-cli/cli/command_framework/kurtosis_command/args"
	"github.com/kurtosis-tech/kurtosis-cli/cli/command_framework/kurtosis_command/flags"
	"github.com/stretchr/testify/require"
	"testing"
)

const (
	arg1Key = "arg1"
	arg2Key = "arg2"
	arg3Key = "arg3"

	arg1Value = "arg1Value"
	arg2Value = "arg2Value"
	arg3Value1 = "arg3Value1"
	arg3Value2 = "arg3Value2"
	arg3Value3 = "arg3Value3"

	flag1Key = "flag1"
	flag2Key = "flag2"
)

var validArgsConfig = []*args.ArgConfig{
	{
		Key: arg1Key,
	},
	{
		Key: arg2Key,
	},
	{
		Key:      arg3Key,
		IsGreedy: true,
	},
}
var validTokens = []string{
	arg1Value,
	arg2Value,
	arg3Value1,
	arg3Value2,
	arg3Value3,
}

func TestMustGetCobraCommand_DuplicateArgsCausePanic(t *testing.T) {
	kurtosisCmd := &KurtosisCommand{
		CommandStr:       "test",
		ShortDescription: "Short description",
		LongDescription:  "This is a very long description",
		Args:             []*args.ArgConfig{
			{
				Key:             arg1Key,
			},
			{
				Key:             arg1Key,
			},
		},
	}

	require.Panics(
		t,
		func() { kurtosisCmd.MustGetCobraCommand() },
		"Expected a panic when trying to supply two args with the same key",
	)
}

func TestMustGetCobraCommand_DuplicateFlagsCausePanic(t *testing.T) {
	kurtosisCmd := &KurtosisCommand{
		CommandStr:       "test",
		ShortDescription: "Short description",
		LongDescription:  "This is a very long description",
		Flags: []*flags.FlagConfig{
			{
				Key: flag1Key,
			},
			{
				Key: flag1Key,
			},
		},
	}

	require.Panics(
		t,
		func() { kurtosisCmd.MustGetCobraCommand() },
		"Expected a panic when trying to supply two flags with the same key",
	)
}

func TestMustGetCobraCommand_FlagsWithMismatchedDefaulValuesCausePanic(t *testing.T) {
	illegalFlagVariants := []*flags.FlagConfig{
		{
			Key:     flag1Key,
			Type:    flags.FlagType_Bool,
			Default: "123",
		},
		{
			Key:     flag1Key,
			Type:    flags.FlagType_Uint32,
			Default: "true",
		},
	}
	for _, illegalFlag := range illegalFlagVariants {
		kurtosisCmd := &KurtosisCommand{
			CommandStr:       "test",
			ShortDescription: "Short description",
			LongDescription:  "This is a very long description",
			Flags: []*flags.FlagConfig{
				illegalFlag,
			},
		}

		require.Panics(
			t,
			func() { kurtosisCmd.MustGetCobraCommand() },
			"Expected a panic when trying to set a flag with type '%v' and default value string '%v' that doesn't match the type",
			illegalFlag.Type.AsString(),
			illegalFlag.Default,
		)
	}
}

func TestMustGetCobraCommand_FlagWithNoTypePanics(t *testing.T) {
	kurtosisCmd := &KurtosisCommand{
		CommandStr:       "test",
		ShortDescription: "Short description",
		LongDescription:  "This is a very long description",
		Flags: []*flags.FlagConfig{
			{
				Key:       flag1Key,
			},
		},
	}

	require.Panics(
		t,
		func() { kurtosisCmd.MustGetCobraCommand() },
		"Expected a panic when a flag has no type",
	)
}

func TestMustGetCobraCommand_FlagWithTypeDoesntPanic(t *testing.T) {
	kurtosisCmd := &KurtosisCommand{
		CommandStr:       "test",
		ShortDescription: "Short description",
		LongDescription:  "This is a very long description",
		Flags: []*flags.FlagConfig{
			{
				Key: flag1Key,
				Type: flags.FlagType_String,
			},
		},
	}

	require.NotPanics(
		t,
		func() { kurtosisCmd.MustGetCobraCommand() },
		"Expected no panic when a flag has a valid type",
	)
}

func TestMustGetCobraCommand_DuplicateFlagShorthandsPanic(t *testing.T) {
	dupedShorthandValue := "x"
	kurtosisCmd := &KurtosisCommand{
		CommandStr:       "test",
		ShortDescription: "Short description",
		LongDescription:  "This is a very long description",
		Flags: []*flags.FlagConfig{
			{
				Key: flag1Key,
				Shorthand: dupedShorthandValue,
			},
			{
				Key: flag2Key,
				Shorthand: dupedShorthandValue,
			},
		},
	}

	require.Panics(
		t,
		func() { kurtosisCmd.MustGetCobraCommand() },
		"Expected a panic when setting two flags with the same shorthand value",
	)
}

func TestMustGetCobraCommand_ShorthandsGreaterThanOneLetterPanic(t *testing.T) {
	kurtosisCmd := &KurtosisCommand{
		CommandStr:       "test",
		ShortDescription: "Short description",
		LongDescription:  "This is a very long description",
		Flags: []*flags.FlagConfig{
			{
				Key: flag1Key,
				Shorthand: "this is way too long",
			},
		},
	}

	require.Panics(
		t,
		func() { kurtosisCmd.MustGetCobraCommand() },
		"Expected a panic when setting a flag whose shorthand is greater than one letter",
	)
}

func TestMustGetCobraCommand_EmptyArgKeyCausesPanic(t *testing.T) {
	kurtosisCmd := &KurtosisCommand{
		CommandStr:       "test",
		ShortDescription: "Short description",
		LongDescription:  "This is a very long description",
		Args: []*args.ArgConfig{
			{
				Key: "  ",
			},
		},
	}

	require.Panics(
		t,
		func() { kurtosisCmd.MustGetCobraCommand() },
		"Expected a panic when trying to set an arg with an empty key",
	)
}

func TestMustGetCobraCommand_EmptyFlagKeyCausesPanic(t *testing.T) {
	kurtosisCmd := &KurtosisCommand{
		CommandStr:       "test",
		ShortDescription: "Short description",
		LongDescription:  "This is a very long description",
		Flags: []*flags.FlagConfig{
			{
				Key: "  ",
			},
		},
	}

	require.Panics(
		t,
		func() { kurtosisCmd.MustGetCobraCommand() },
		"Expected a panic when trying to set a flag with an empty key",
	)
}

func TestMustGetCobraCommand_TwoOptionalArgumentsCausePanic(t *testing.T) {
	kurtosisCmd := &KurtosisCommand{
		CommandStr:       "test",
		ShortDescription: "Short description",
		LongDescription:  "This is a very long description",
		Args:             []*args.ArgConfig{
			{
				Key:             arg1Key,
			},
			{
				Key:             arg2Key,
				IsOptional: true,
			},
			{
				Key:             arg3Key,
				IsOptional: true,
			},
		},
	}

	require.Panics(
		t,
		func() { kurtosisCmd.MustGetCobraCommand() },
		"Expected a panic when trying to supply two optional commands, but none happened",
	)
}

func TestMustGetCobraCommand_MiddleGreedyArgCausesPanic(t *testing.T) {
	kurtosisCmd := &KurtosisCommand{
		CommandStr:       "test",
		ShortDescription: "Short description",
		LongDescription:  "This is a very long description",
		Args:             []*args.ArgConfig{
			{
				Key:             arg1Key,
			},
			{
				Key:             arg2Key,
				IsGreedy: true,
			},
			{
				Key:             arg3Key,
			},
		},
	}

	require.Panics(
		t,
		func() { kurtosisCmd.MustGetCobraCommand() },
		"Expected a panic when trying to supply a greedy argument with another argument after it",
	)
}

func TestMustGetCobraCommand_WorkingFlagDefaultValueChecking(t *testing.T) {
	kurtosisCmd := &KurtosisCommand{
		CommandStr:       "test",
		ShortDescription: "Short description",
		LongDescription:  "This is a very long description",
		Flags: []*flags.FlagConfig{
			{
				Key:     flag1Key,
				Type:    flags.FlagType_Uint32,
				Default: "0",
			},
			{
				Key:     flag2Key,
				Type:    flags.FlagType_Bool,
				Default: "false",
			},
		},
	}

	require.NotPanics(
		t,
		func() { kurtosisCmd.MustGetCobraCommand() },
		"Expected default value flag validation to work",
	)
}
