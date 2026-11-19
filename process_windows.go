package main

import (
	"os/exec"
	"strconv"
)

func prepare(c *exec.Cmd) {}
func terminate(c *exec.Cmd) error {
	return exec.Command("taskkill.exe", "/PID", strconv.Itoa(c.Process.Pid), "/T", "/F").Run()
}
