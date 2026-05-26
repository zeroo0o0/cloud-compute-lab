package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	target := "student"
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "student" {
		target = args[0]
		args = args[1:]
	}

	testDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取当前目录失败：%v\n", err)
		os.Exit(1)
	}
	labDir := filepath.Clean(filepath.Join(testDir, ".."))
	srcDir := filepath.Join(labDir, target)
	cacheDir := filepath.Join(labDir, ".gocache")
	_ = os.MkdirAll(cacheDir, 0o755)

	cmdArgs := append([]string{"run", "autotest.go"}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Dir = testDir
	cmd.Env = append(os.Environ(), "GOCACHE="+cacheDir, "LAB3_SRC_DIR="+srcDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}
