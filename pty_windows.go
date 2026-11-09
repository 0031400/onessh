//go:build windows

package main

import (
	"os"
	"os/exec"

	"golang.org/x/crypto/ssh"
)

// defaultShell returns cmd.exe on Windows.
func defaultShell() string {
	if sh := os.Getenv("COMSPEC"); sh != "" {
		return sh
	}
	return "cmd.exe"
}

// runWithPty falls back to plain pipes on Windows (no ConPTY integration).
func runWithPty(cmd *exec.Cmd, ch ssh.Channel, term string, cols, rows int, winch <-chan [2]int) {
	// 消费 resize 事件，避免发送方阻塞（Windows 下不支持 PTY 调整）
	go func() {
		for range winch {
		}
	}()
	runWithPipes(cmd, ch)
}
