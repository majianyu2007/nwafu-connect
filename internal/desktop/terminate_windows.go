//go:build windows

package desktop

import (
	"fmt"
	"os/exec"
	"time"
)

func terminateProcess(command *exec.Cmd, done <-chan struct{}) {
	// Ask the core process tree to exit gracefully first so it can flush
	// cleanup hooks and remove temporary browser profiles cleanly.
	_ = exec.Command("taskkill.exe", "/PID", fmt.Sprint(command.Process.Pid), "/T").Run()
	select {
	case <-done:
		return
	case <-time.After(2500 * time.Millisecond):
	}
	_ = exec.Command("taskkill.exe", "/PID", fmt.Sprint(command.Process.Pid), "/T", "/F").Run()
	select {
	case <-done:
	case <-time.After(2500 * time.Millisecond):
		_ = command.Process.Kill()
		<-done
	}
}
