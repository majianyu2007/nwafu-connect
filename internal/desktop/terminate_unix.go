//go:build !windows

package desktop

import (
	"os"
	"os/exec"
	"time"
)

func terminateProcess(command *exec.Cmd, done <-chan struct{}) {
	_ = command.Process.Signal(os.Interrupt)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		<-done
	}
}
