//go:build !(aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris)

package environment

import "os/exec"

// configureTargetCommandCancellation bounds waiting on inherited output pipes
// when the platform cannot terminate a Unix process group.
func configureTargetCommandCancellation(cmd *exec.Cmd) {
	cmd.WaitDelay = targetCommandWaitDelay
}
