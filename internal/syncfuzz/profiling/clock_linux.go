//go:build linux

package profiling

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func monotonicNowNS() (uint64, error) {
	var value unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &value); err != nil {
		return 0, fmt.Errorf("read CLOCK_MONOTONIC: %w", err)
	}
	return uint64(value.Sec)*1_000_000_000 + uint64(value.Nsec), nil
}
