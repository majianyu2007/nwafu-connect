//go:build windows

package desktop

import (
	"fmt"
	"os/exec"
	"time"
)

func terminateProcess(command *exec.Cmd, done <-chan struct{}) {
	_ = exec.Command("taskkill.exe", "/PID", fmt.Sprint(command.Process.Pid), "/T", "/F").Run()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		<-done
	}
}
