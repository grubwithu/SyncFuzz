//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package environment

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// configureTargetCommandCancellation makes the command process the leader of
// a new group so a context timeout also terminates shell-launched descendants.
func configureTargetCommandCancellation(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = targetCommandWaitDelay
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		return nil
	}
}
