package command

import "os/exec"

func buildFalseCmd() *exec.Cmd { return exec.Command("false") }
func buildTrueCmd() *exec.Cmd  { return exec.Command("true") }
