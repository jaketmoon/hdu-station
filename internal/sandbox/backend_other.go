//go:build !darwin && !windows

package sandbox

func newPlatformExecutor(_ string, _ string, _ string, _ ImageSource, status Status) Executor {
	return &unavailableExecutor{status: status}
}
