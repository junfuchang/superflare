//go:build !linux && !windows

package ports

func collectRuntimePorts() []runtimePort {
	return nil
}
