package docker_manager

import (
	"github.com/docker/go-connections/nat"
	"net"
)

// See CreateAndStartContainerArgsBuilder for detailed documentation on the fields
type CreateAndStartContainerArgs struct {
	dockerImage                    string
	name                           string
	alias                          string
	interactiveModeTtySize         *InteractiveModeTtySize // If nil interactive mode will be disabled; if non-nil then interactive mode will be enabled
	networkId                      string
	staticIp                       net.IP
	addedCapabilities              map[ContainerCapability]bool
	networkMode                    DockerManagerNetworkMode
	usedPorts                      map[nat.Port]PortPublishSpec
	entrypointArgs                 []string
	cmdArgs                        []string
	envVariables                   map[string]string
	bindMounts                     map[string]string
	volumeMounts                   map[string]string
	needsAccessToDockerHostMachine bool
	labels                         map[string]string
	cpuAllocation                  string
	memoryAllocation               uint64
}

// Builder for creating CreateAndStartContainerArgs object
type CreateAndStartContainerArgsBuilder struct {
	dockerImage                    string
	name                           string
	alias                          string
	interactiveModeTtySize         *InteractiveModeTtySize // If nil interactive mode will be disabled; if non-nil then interactive mode will be enabled
	networkId                      string
	staticIp                       net.IP
	addedCapabilities              map[ContainerCapability]bool
	networkMode                    DockerManagerNetworkMode
	usedPorts                      map[nat.Port]PortPublishSpec
	entrypointArgs                 []string
	cmdArgs                        []string
	envVariables                   map[string]string
	bindMounts                     map[string]string
	volumeMounts                   map[string]string
	needsAccessToDockerHostMachine bool
	labels                         map[string]string
	cpuAllocation                  string
	memoryAllocation               uint64
}

/*
Args:
	dockerImage: Image to start
	name: The name to give the container to be created
	networkId: The ID of the Docker network that this container should be attached to
*/
func NewCreateAndStartContainerArgsBuilder(dockerImage string, name string, networkId string) *CreateAndStartContainerArgsBuilder {
	return &CreateAndStartContainerArgsBuilder{
		dockerImage:                    dockerImage,
		name:                           name,
		alias:                          "",
		interactiveModeTtySize:         nil,
		networkId:                      networkId,
		staticIp:                       nil,
		addedCapabilities:              map[ContainerCapability]bool{},
		networkMode:                    DefaultNetworkMode,
		usedPorts:                      map[nat.Port]PortPublishSpec{},
		entrypointArgs:                 nil,
		cmdArgs:                        nil,
		envVariables:                   map[string]string{},
		bindMounts:                     map[string]string{},
		volumeMounts:                   map[string]string{},
		needsAccessToDockerHostMachine: false,
		labels:                         map[string]string{},
		cpuAllocation:                  "",
		memoryAllocation:               0,
	}
}

func (builder *CreateAndStartContainerArgsBuilder) Build() *CreateAndStartContainerArgs {
	return &CreateAndStartContainerArgs{
		dockerImage:                    builder.dockerImage,
		name:                           builder.name,
		labels:                         builder.labels,
		alias:                          builder.alias,
		interactiveModeTtySize:         builder.interactiveModeTtySize,
		networkId:                      builder.networkId,
		staticIp:                       builder.staticIp,
		addedCapabilities:              builder.addedCapabilities,
		networkMode:                    builder.networkMode,
		usedPorts:                      builder.usedPorts,
		entrypointArgs:                 builder.entrypointArgs,
		cmdArgs:                        builder.cmdArgs,
		envVariables:                   builder.envVariables,
		bindMounts:                     builder.bindMounts,
		volumeMounts:                   builder.volumeMounts,
		needsAccessToDockerHostMachine: builder.needsAccessToDockerHostMachine,
		cpuAllocation:                  builder.cpuAllocation,
		memoryAllocation:               builder.memoryAllocation,
	}
}

// Alias to give the container, so that other machines can reference the container by this alias to connect to it
func (builder *CreateAndStartContainerArgsBuilder) WithAlias(alias string) *CreateAndStartContainerArgsBuilder {
	builder.alias = alias
	return builder
}

// If non-nil, the container will be started in interactive mode, with a container TTY
//  set to the specified dimensions
func (builder *CreateAndStartContainerArgsBuilder) WithInteractiveModeTtySize(size *InteractiveModeTtySize) *CreateAndStartContainerArgsBuilder {
	builder.interactiveModeTtySize = size
	return builder
}

// IP the container will be assigned (leave nil to not assign any IP, which only works with the bridge network)
func (builder *CreateAndStartContainerArgsBuilder) WithStaticIP(ip net.IP) *CreateAndStartContainerArgsBuilder {
	builder.staticIp = ip
	return builder
}

// A "set" of capabilities to add to the container, corresponding to the --cap-add Docker flag
// For more info, see the --cap-add section of https://docs.docker.com/engine/reference/run/
func (builder *CreateAndStartContainerArgsBuilder) WithAddedCapabilities(capabilities map[ContainerCapability]bool) *CreateAndStartContainerArgsBuilder {
	builder.addedCapabilities = capabilities
	return builder
}

// When a non-empty string, sets the Docker --network flag to be this given string
func (builder *CreateAndStartContainerArgsBuilder) WithNetworkMode(mode DockerManagerNetworkMode) *CreateAndStartContainerArgsBuilder {
	builder.networkMode = mode
	return builder
}

// A set of ports that the container will listen on, mapped to a specification for how they should be published to the host machine (if at all)
func (builder *CreateAndStartContainerArgsBuilder) WithUsedPorts(usedPorts map[nat.Port]PortPublishSpec) *CreateAndStartContainerArgsBuilder {
	builder.usedPorts = usedPorts
	return builder
}

// The args that will be used to override the ENTRYPOINT of the image (leave as nil to not override)
func (builder *CreateAndStartContainerArgsBuilder) WithEntrypointArgs(args []string) *CreateAndStartContainerArgsBuilder {
	builder.entrypointArgs = args
	return builder
}

// The args that will be used to run the container (leave as nil to run the CMD in the image)
func (builder *CreateAndStartContainerArgsBuilder) WithCmdArgs(args []string) *CreateAndStartContainerArgsBuilder {
	builder.cmdArgs = args
	return builder
}

// A key-value mapping of Docker environment variables which will be passed to the container during startup
func (builder *CreateAndStartContainerArgsBuilder) WithEnvironmentVariables(envVars map[string]string) *CreateAndStartContainerArgsBuilder {
	builder.envVariables = envVars
	return builder
}

// Mapping of (host file) -> (mountpoint on container) that will be mounted on container startup
func (builder *CreateAndStartContainerArgsBuilder) WithBindMounts(bindMounts map[string]string) *CreateAndStartContainerArgsBuilder {
	builder.bindMounts = bindMounts
	return builder
}

// Mounts: Mapping of (volume name) -> (mountpoint on container) to mount during container launch
func (builder *CreateAndStartContainerArgsBuilder) WithVolumeMounts(volumeMounts map[string]string) *CreateAndStartContainerArgsBuilder {
	builder.volumeMounts = volumeMounts
	return builder
}

// Will provide the container with a magic "host.docker.internal" domain name
//  that it can use to access ports of the machine running Docker itself (useful if, e.g., the container
//  needs to check the host machine's free ports)
func (builder *CreateAndStartContainerArgsBuilder) NeedsAccessToDockerHostMachine(needsAccess bool) *CreateAndStartContainerArgsBuilder {
	builder.needsAccessToDockerHostMachine = needsAccess
	return builder
}

// A key-value map that represents labels to give the container, for use in searching later
func (builder *CreateAndStartContainerArgsBuilder) WithLabels(labels map[string]string) *CreateAndStartContainerArgsBuilder {
	builder.labels = labels
	return builder
}

// Corresponds to `--cpus` limit in Docker, converted to NanoCPUs set in the underlying container
// The empty string is the empty value, meaning if the value is "", this field is ignored
// https://pkg.go.dev/github.com/docker/docker@v20.10.17+incompatible/api/types/container#Resources
func (builder *CreateAndStartContainerArgsBuilder) WithCPUAllocation(cpuAllocation string) *CreateAndStartContainerArgsBuilder {
	builder.cpuAllocation = cpuAllocation
	return builder
}

// Corresponds to `--memory` limit in Docker in megabytes, used to set Memory and MemorySwap resource in the underlying container
// 0 is the empty value, meaning if the value is 0, this field is ignored
// https://pkg.go.dev/github.com/docker/docker@v20.10.17+incompatible/api/types/container#Resources
func (builder *CreateAndStartContainerArgsBuilder) WithMemoryAllocation(memoryAllocation uint64) *CreateAndStartContainerArgsBuilder {
	builder.memoryAllocation = memoryAllocation
	return builder
}
