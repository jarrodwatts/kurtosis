/*
 * Copyright (c) 2021 - present Kurtosis Technologies Inc.
 * All Rights Reserved.
 */

package service_ip_replacer

import (
	"fmt"
	"github.com/kurtosis-tech/kurtosis-core/server/api_container/server/service_network"
	"github.com/kurtosis-tech/kurtosis-core/server/api_container/server/service_network/service_network_types"
	"github.com/kurtosis-tech/stacktrace"
	"net"
	"regexp"
	"strings"
)

const (
	considerEntireStringOffset = -1
)

/**
This struct replaces instances of PREFIX + service_id + SUFFIX in a string with the IP address of the referenced service ID
 */
type ServiceIPReplacer struct {
	serviceIdReplaceStrPrefix string // Prefix of the service ID replace string, which looks like PREFIX + service_id + SUFFIX
	serviceIdReplaceStrSuffix string // Suffix of the service ID replace string, which looks like PREFIX + service_id + SUFFIX
	serviceIdReplaceStrMatchPattern *regexp.Regexp // Compiled regex pattern matching the service ID -> IP replacement string
	serviceNetwork service_network.ServiceNetwork
}

func NewServiceIPReplacer(serviceIdReplaceStrPrefix string, serviceIdReplaceStrSuffix string, serviceNetwork service_network.ServiceNetwork) (*ServiceIPReplacer, error) {
	if serviceIdReplaceStrPrefix == serviceIdReplaceStrSuffix {
		return nil, stacktrace.NewError("Service ID replace pattern prefix '%v' is the same as the suffix", serviceIdReplaceStrPrefix)
	}
	if len(serviceIdReplaceStrPrefix) == 0 || len(serviceIdReplaceStrSuffix) == 0 {
		return nil, stacktrace.NewError("Service ID replacement pattern prefix & suffix must both have >= 1 character")
	}

	// The *? means a non-greedy match
	matchPatternStr := fmt.Sprintf("%v.*?%v", serviceIdReplaceStrPrefix, serviceIdReplaceStrSuffix)
	compiledMatchPattern, err := regexp.Compile(matchPatternStr)
	if err != nil {
		return nil, stacktrace.Propagate(err, "An error occurred compiling service ID match pattern '%v'", matchPatternStr)
	}
	return &ServiceIPReplacer{
		serviceIdReplaceStrPrefix: serviceIdReplaceStrPrefix,
		serviceIdReplaceStrSuffix: serviceIdReplaceStrSuffix,
		serviceIdReplaceStrMatchPattern: compiledMatchPattern,
		serviceNetwork: serviceNetwork,
	}, nil
}

// Returns a copy of the string with all referenced instances of service IDs replaced with their IP address
func (replacer ServiceIPReplacer) ReplaceStr(str string) (string, error) {
	matches := replacer.serviceIdReplaceStrMatchPattern.FindAllString(str, considerEntireStringOffset)
	if matches == nil {
		return str, nil
	}

	ipReplacementRegexes := map[string]net.IP{} // Maps regex replacement pattern -> service IP
	for _, matchStr := range matches {
		matchStrWithoutPrefix := strings.TrimPrefix(matchStr, replacer.serviceIdReplaceStrPrefix)
		serviceId := strings.TrimSuffix(matchStrWithoutPrefix, replacer.serviceIdReplaceStrSuffix)
		privateIp, _, err := replacer.serviceNetwork.GetServiceRegistrationInfo(service_network_types.ServiceID(serviceId))
		if err != nil {
			return "", stacktrace.Propagate(
				err,
				"An error occurred getting private IP address for service '%v', requested by replacement string '%v'",
				serviceId,
				matchStr,
			)
		}
		ipReplacementRegexes[matchStr] = privateIp
	}

	result := str
	for matchStr, ip := range ipReplacementRegexes {
		result = strings.ReplaceAll(result, matchStr, ip.String())
	}
	return result, nil
}

func (replacer ServiceIPReplacer) ReplaceStrSlice(strSlice []string) ([]string, error) {
	result := []string{}
	for idx, elem := range strSlice {
		replaced, err := replacer.ReplaceStr(elem)
		if err != nil {
			return nil, stacktrace.Propagate(err, "An error occurred replacing service IDs with IPs in string slice element #%v '%v'", idx, elem)
		}
		result = append(result, replaced)
	}
	return result, nil
}

func (replacer ServiceIPReplacer) ReplaceMapValues(input map[string]string) (map[string]string, error) {
	result := map[string]string{}
	for key, val := range input {
		replacedVal, err := replacer.ReplaceStr(val)
		if err != nil {
			return nil, stacktrace.Propagate(err, "An error occurred replacing service IDs with IPs in map value '%v' corresponding to key '%v'", val, key)
		}
		result[key] = replacedVal
	}
	return result, nil
}
