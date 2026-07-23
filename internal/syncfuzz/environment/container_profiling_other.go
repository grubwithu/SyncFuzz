//go:build !linux

package environment

import (
	"context"
	"fmt"

	"github.com/grubwithu/syncfuzz/internal/syncfuzz/profiling"
)

func ResolveContainerProfilingScope(_ context.Context, _ string, _ string) (profiling.ProfilingScope, error) {
	return profiling.ProfilingScope{}, fmt.Errorf("container profiling scope requires Linux cgroup v2")
}
