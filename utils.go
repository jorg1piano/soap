package main

import (
	"os/exec"
	"strings"
)

// runBashCommand executes a bash command and returns trimmed output
func runBashCommand(cmdStr string) (string, error) {
	cmd := exec.Command("bash", "-c", cmdStr)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// runBashCommandInDir executes a bash command in a specific directory
func runBashCommandInDir(cmdStr, dir string) error {
	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Dir = dir
	return cmd.Run()
}

// cleanupWorktree removes a git worktree
func cleanupWorktree(path string) {
	if path == "" {
		return
	}
	cmd := exec.Command("git", "worktree", "remove", "--force", path)
	cmd.Run()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
