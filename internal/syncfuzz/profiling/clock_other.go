//go:build !linux

package profiling

import "fmt"

func monotonicNowNS() (uint64, error) {
	return 0, fmt.Errorf("CLOCK_MONOTONIC checkpoint timestamps require Linux")
}
