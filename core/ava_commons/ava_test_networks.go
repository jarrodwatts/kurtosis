package ava_commons

import "github.com/gmarchetti/kurtosis/commons"

type SingleNodeAvaNetworkCfgProvider struct{
	GeckoImageName string
}
func (network SingleNodeAvaNetworkCfgProvider) GetNetworkConfig() *commons.JsonRpcServiceNetworkConfig {
	// TODO set up non-null nodes (indicating that they're not boot nodes)
	bootNodes := make(map[commons.JsonRpcServiceSocket]commons.JsonRpcRequest)
	geckoNodeConfig := NewGeckoServiceConfig(network.GeckoImageName, bootNodes)

	builder := commons.NewJsonRpcServiceNetworkConfigBuilder()
	builder.AddNode(geckoNodeConfig, make(map[int]bool))
	return builder.Build()
}
