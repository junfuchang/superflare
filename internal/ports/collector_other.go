//go:build !linux && !windows

package ports

func collectRuntimePorts() []runtimePort {
	return unsupportedRuntimeCollectorResult().Ports
}

func collectRuntimePortsResult() runtimeCollectorResult {
	return unsupportedRuntimeCollectorResult()
}
