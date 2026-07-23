//go:build !linux

package environment

func readProcFDIdentity(fdPath string) (uint64, uint64) {
	return 0, 0
}
