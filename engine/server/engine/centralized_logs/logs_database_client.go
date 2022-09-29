package centralized_logs

import (
	"context"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/enclave"
	"github.com/kurtosis-tech/kurtosis/container-engine-lib/lib/backend_interface/objects/service"
)

type LogsDatabaseClient interface {
	GetUserServiceLogs(
		ctx context.Context,
		enclaveID enclave.EnclaveID,
		userServiceGuids map[service.ServiceGUID]bool,
	) (
		resultUserServiceLogsByServiceGUID map[service.ServiceGUID][]string,
		resultErr error,
	)
}
