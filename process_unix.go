//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

func prepare(c *exec.Cmd)         { c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func terminate(c *exec.Cmd) error { return syscall.Kill(-c.Process.Pid, syscall.SIGTERM) }
