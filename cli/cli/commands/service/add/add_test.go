package add

import (
	"fmt"
	"github.com/kurtosis-tech/kurtosis/api/golang/core/lib/services"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestParsePortSpecstr_SuccessCases(t *testing.T) {
	type args struct {
		specStr string
	}

	parsePortSpecSuccessTests := []struct {
		name string
		args args
		want *services.PortSpec
	}{
		{
			name: "Successfully parse str with application protocol and without transport protocol",
			args: args{
				specStr: "http:3333",
			},
			want: services.NewPortSpec(uint16(3333), services.TransportProtocol_TCP, "http"),
		},
		{
			name: "Successfully parse str with application protocol and with transport protocol",
			args: args{
				specStr: "http:3333/udp",
			},
			want: services.NewPortSpec(uint16(3333), services.TransportProtocol_UDP, "http"),
		},
		{
			name: "Successfully parse str without application protocol and with transport protocol",
			args: args{
				specStr: "3333/udp",
			},
			want: services.NewPortSpec(uint16(3333), services.TransportProtocol_UDP, ""),
		},
		{
			name: "Successfully parse str without application protocol and without transport protocol",
			args: args{
				specStr: "3333",
			},
			want: services.NewPortSpec(uint16(3333), services.TransportProtocol_TCP, ""),
		},
	}
	for _, parsePortSpecTest := range parsePortSpecSuccessTests {
		t.Run(parsePortSpecTest.name, func(t *testing.T) {
			got, err := parsePortSpecStr(parsePortSpecTest.args.specStr)
			require.NoError(t, err, "Unexpected error occurred while testing")
			require.Equal(t, parsePortSpecTest.want, got)
		})
	}
}

func TestParsePortSpecstr_FailureCases(t *testing.T) {
	type args struct {
		specStr string
	}
	parsePortSpecFailureTests := []struct {
		name       string
		args       args
		errMessage string
	}{
		{
			name: "Failure while parsing, missing port number",
			args: args{
				specStr: "http:tcp",
			},
			errMessage: fmt.Sprintf("Error occurred while parsing port number '%v' in port spec '%v'", "tcp", "http:tcp"),
		},
		{
			name: "Failure while parsing, more than one delimiter ':'",
			args: args{
				specStr: "http:233:80",
			},
			errMessage: fmt.Sprintf("Error occurred while parsing port number '%v' in port spec '%v'", "233:80", "http:233:80"),
		},
		{
			name: "Failure while parsing, empty application protocol",
			args: args{
				specStr: ":3333/udp",
			},
			errMessage: fmt.Sprintf("Error occurred while parsing application protocol '%v' in port spec '%v'", "", ":3333/udp"),
		},
		{
			name: "Failure while parsing, extra delimeter(:) is present",
			args: args{
				specStr: "http:80/udp:",
			},
			errMessage: fmt.Sprintf("Error occurred while parsing transport protocol '%v' in port spec '%v'", "udp:", "http:80/udp:"),
		},
		{
			name: "Failure while parsing, port number is not a number",
			args: args{
				specStr: "http:abc/udp",
			},
			errMessage: fmt.Sprintf("Error occurred while parsing port number '%v' in port spec '%v'", "abc", "http:abc/udp"),
		},
	}
	for _, parsePortSpecTest := range parsePortSpecFailureTests {
		t.Run(parsePortSpecTest.name, func(t *testing.T) {
			_, err := parsePortSpecStr(parsePortSpecTest.args.specStr)
			require.NotNil(t, err, "Expected error, but received nil")
			require.ErrorContains(t, err, parsePortSpecTest.errMessage)
		})
	}
}

func TestParsePortSpecstr_EmptyIsError(t *testing.T) {
	_, err := parsePortSpecStr("")
	require.Error(t, err)
}

func TestParsePortSpecstr_AlphabeticalIsError(t *testing.T) {
	_, err := parsePortSpecStr("abd")
	require.Error(t, err)
}

func TestParsePortSpecstr_TooManyDelimsIsError(t *testing.T) {
	_, err := parsePortSpecStr("1234/tcp/udp")
	require.Error(t, err)
}

func TestParsePortSpecstr_DefaultTcpProtocol(t *testing.T) {
	portSpec, err := parsePortSpecStr("1234")
	require.NoError(t, err)
	require.Equal(t, uint16(1234), portSpec.GetNumber())
	require.Equal(t, services.TransportProtocol_TCP, portSpec.GetTransportProtocol())
}

func TestParsePortSpecstr_CustomProtocol(t *testing.T) {
	portSpec, err := parsePortSpecStr("1234/udp")
	require.NoError(t, err)
	require.Equal(t, uint16(1234), portSpec.GetNumber())
	require.Equal(t, services.TransportProtocol_UDP, portSpec.GetTransportProtocol())
}

func TestParsePortsStr_DuplicatePortsCauseError(t *testing.T) {
	_, err := parsePortsStr("http=80/tcp,http=8080")
	require.Error(t, err)
}

func TestParsePortsStr_EmptyPortIDCausesError(t *testing.T) {
	_, err := parsePortsStr("=80/tcp")
	require.Error(t, err)
}

func TestParsePortsStr_SuccessfulPortsString(t *testing.T) {
	ports, err := parsePortsStr("port1=8080,port2=2900/udp")
	require.NoError(t, err)
	require.Equal(t, 2, len(ports))

	port1Spec, found := ports["port1"]
	require.True(t, found)
	require.Equal(t, uint16(8080), port1Spec.GetNumber())
	require.Equal(t, services.TransportProtocol_TCP, port1Spec.GetTransportProtocol())

	port2Spec, found := ports["port2"]
	require.True(t, found)
	require.Equal(t, uint16(2900), port2Spec.GetNumber())
	require.Equal(t, services.TransportProtocol_UDP, port2Spec.GetTransportProtocol())
}

func TestParseEnvVarsStr_EqualSignInValueIsOkay(t *testing.T) {
	envvars, err := parseEnvVarsStr("VAR=thing=otherthing")
	require.NoError(t, err)
	require.Equal(t, 1, len(envvars))

	value, found := envvars["VAR"]
	require.True(t, found)
	require.Equal(t, "thing=otherthing", value)
}

func TestParseEnvVarsStr_MultipleVarsAreOkay(t *testing.T) {
	envvars, err := parseEnvVarsStr("VAR1=VALUE1,VAR2=VALUE2")
	require.NoError(t, err)
	require.Equal(t, 2, len(envvars))

	value1, found := envvars["VAR1"]
	require.True(t, found)
	require.Equal(t, "VALUE1", value1)

	value2, found := envvars["VAR2"]
	require.True(t, found)
	require.Equal(t, "VALUE2", value2)
}

func TestParseEnvVarsStr_DuplicateVarNamesError(t *testing.T) {
	_, err := parseEnvVarsStr("VAR1=VALUE1,VAR1=VALUE2")
	require.Error(t, err)
}

func TestParseEnvVarsStr_EmptyDeclarations(t *testing.T) {
	envvars, err := parseEnvVarsStr("VAR1=VALUE1,, ,  ,,")
	require.NoError(t, err)
	require.Equal(t, 1, len(envvars))
}

func TestParseFilesArtifactMountStr_ValidParse(t *testing.T) {
	artifactUuid1 := services.FilesArtifactUUID("1234")
	artifactUuid2 := services.FilesArtifactUUID("4567")
	mountpoint1 := "/dest1"
	mountpoint2 := "/dest2"

	result, err := parseFilesArtifactMountsStr(fmt.Sprintf(
		"%v:%v,%v:%v",
		artifactUuid1,
		mountpoint1,
		artifactUuid2,
		mountpoint2,
	))
	require.NoError(t, err)
	require.Equal(t, 2, len(result))

	parsedMountpoint1, found := result[artifactUuid1]
	require.True(t, found)
	require.Equal(t, mountpoint1, parsedMountpoint1)

	parsedMountpoint2, found := result[artifactUuid2]
	require.True(t, found)
	require.Equal(t, mountpoint2, parsedMountpoint2)
}

func TestParseFilesArtifactMountStr_EmptyDeclarationsAreSkipped(t *testing.T) {
	artifactUuid1 := services.FilesArtifactUUID("1234")
	artifactUuid2 := services.FilesArtifactUUID("4567")
	mountpoint1 := "/dest1"
	mountpoint2 := "/dest2"

	result, err := parseFilesArtifactMountsStr(fmt.Sprintf(
		"%v:%v,,,,,%v:%v",
		artifactUuid1,
		mountpoint1,
		artifactUuid2,
		mountpoint2,
	))
	require.NoError(t, err)
	require.Equal(t, 2, len(result))

	parsedMountpoint1, found := result[artifactUuid1]
	require.True(t, found)
	require.Equal(t, mountpoint1, parsedMountpoint1)

	parsedMountpoint2, found := result[artifactUuid2]
	require.True(t, found)
	require.Equal(t, mountpoint2, parsedMountpoint2)
}

func TestParseFilesArtifactMountStr_TooManyArtifactUuidMountpointDelimitersIsError(t *testing.T) {
	artifactUuid := services.FilesArtifactUUID("1234")
	mountpoint := "/dest"

	_, err := parseFilesArtifactMountsStr(fmt.Sprintf(
		"%v::%v",
		artifactUuid,
		mountpoint,
	))
	require.Error(t, err)
}

func TestParseFilesArtifactMountStr_TooFewArtifactUuidMountpointDelimitersIsError(t *testing.T) {
	artifactUuid := services.FilesArtifactUUID("1234")
	mountpoint := "/dest"

	_, err := parseFilesArtifactMountsStr(fmt.Sprintf(
		"%v%v",
		artifactUuid,
		mountpoint,
	))
	require.Error(t, err)
}

func TestParseFilesArtifactMountStr_DuplicateArtifactUuids(t *testing.T) {
	artifactUuid := services.FilesArtifactUUID("1234")
	mountpoint1 := "/dest1"
	mountpoint2 := "/dest2"

	_, err := parseFilesArtifactMountsStr(fmt.Sprintf(
		"%v:%v,%v:%v",
		artifactUuid,
		mountpoint1,
		artifactUuid,
		mountpoint2,
	))
	require.Error(t, err)
}
