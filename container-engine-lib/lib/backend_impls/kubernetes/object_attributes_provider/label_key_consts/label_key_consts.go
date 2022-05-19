package label_key_consts

import (
	"github.com/kurtosis-tech/container-engine-lib/lib/backend_impls/kubernetes/object_attributes_provider/kubernetes_label_key"
)

const (
	// !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!! DO NOT CHANGE THESE VALUES !!!!!!!!!!!!!!!!!!!!!!!!!!!!!
	// If these value change, it will lead to the Kurtosis engine losing track of old containers
	// which will cause a resource leak on the user's system!
	//
	// If you add new immutable values to this section, MAKE SURE TO UPDATE THE UNIT TEST!
	// These immutable values track resources between Kurtosis versions.
	labelKeyPrefixStr       	= "kurtosistech.com/"
	appIdLabelKeyStr        	= labelKeyPrefixStr + "app-id"
	resourceTypeLabelKeyStr 	= labelKeyPrefixStr + "resource-type"
	// !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!! DO NOT CHANGE THESE VALUES !!!!!!!!!!!!!!!!!!!!!!!!!!!!!

	containerTypeLabelKeyStr	= labelKeyPrefixStr + "container-type"
	volumeTypeLabelKeyStr		= labelKeyPrefixStr + "volume-type"

	// A label to identify a Kurtosis resource (e.g. network, container, etc.) by its id
	idLabelKeyStr = labelKeyPrefixStr + "id"

	// Used for things like service GUID, module GUID, etc.
	guidLabelKeyStr = labelKeyPrefixStr + "guid"

	portSpecsLabelKeyStr = labelKeyPrefixStr + "ports"

	enclaveIdLabelKeyStr = labelKeyPrefixStr + "enclave-id"

	isNetworkPartitioningEnabledKeyStr = labelKeyPrefixStr + "is-network-partitioning-enabled"

	// TODO Remove this and instead use container domain names so we're not dependent on static IPs!
	privateIpAddrLabelKeyStr = labelKeyPrefixStr + "private-ip"
)

// !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!! DO NOT CHANGE THESE VALUES !!!!!!!!!!!!!!!!!!!!!!!!!!!!!
// If these value change, it will lead to the Kurtosis engine losing track of old resources
//  which will cause a resource leak on the user's cluster!
//
//   If you add new immutable values to this section, MAKE SURE TO UPDATE THE UNIT TEST!
//
var AppIDKubernetesLabelKey = kubernetes_label_key.MustCreateNewKubernetesLabelKey(appIdLabelKeyStr)
var KurtosisResourceTypeKubernetesLabelKey = kubernetes_label_key.MustCreateNewKubernetesLabelKey(resourceTypeLabelKeyStr)
// !!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!! DO NOT CHANGE THESE VALUES !!!!!!!!!!!!!!!!!!!!!!!!!!!!!

var KurtosisContainerTypeKubernetesLabelKey = kubernetes_label_key.MustCreateNewKubernetesLabelKey(containerTypeLabelKeyStr)
var KurtosisVolumeTypeKubernetesLabelKey = kubernetes_label_key.MustCreateNewKubernetesLabelKey(volumeTypeLabelKeyStr)
var IDKubernetesLabelKey = kubernetes_label_key.MustCreateNewKubernetesLabelKey(idLabelKeyStr)
var GUIDKubernetesLabelKey = kubernetes_label_key.MustCreateNewKubernetesLabelKey(guidLabelKeyStr)
var PortSpecsKubernetesLabelKey = kubernetes_label_key.MustCreateNewKubernetesLabelKey(portSpecsLabelKeyStr)
var EnclaveIDKubernetesLabelKey = kubernetes_label_key.MustCreateNewKubernetesLabelKey(enclaveIdLabelKeyStr)
var IsNetworkPartitioningEnabledKubernetesLabelKey = kubernetes_label_key.MustCreateNewKubernetesLabelKey(isNetworkPartitioningEnabledKeyStr)
var PrivateIPKubernetesLabelKey = kubernetes_label_key.MustCreateNewKubernetesLabelKey(privateIpAddrLabelKeyStr)
