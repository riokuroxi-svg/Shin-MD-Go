// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//go:build linux

package musica

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

// spotDL puede abrir ffmpeg y proveedores auxiliares. Al vencer el contexto se
// termina el grupo completo, no solo el proceso padre que los lanzó.
func configureSpotDLProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 2 * time.Second
}
