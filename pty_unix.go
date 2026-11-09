//go:build !windows

package main

import (
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
	"golang.org/x/crypto/ssh"
)

// defaultShell returns the user's shell or /bin/sh.
func defaultShell() string {
	if sh := os.Getenv("SHELL"); sh != "" {
		return sh
	}
	return "/bin/sh"
}

// runWithPty runs cmd attached to a pseudo-terminal, bridging it to the SSH
// channel. It applies the client's terminal size and honours later resizes.
func runWithPty(cmd *exec.Cmd, ch ssh.Channel, term string, cols, rows int, winch <-chan [2]int) {
	cmd.Env = append(cmd.Env, "TERM="+term)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{
		Cols: uint16(cols),
		Rows: uint16(rows),
	})
	if err != nil {
		io.WriteString(ch.Stderr(), err.Error()+"\n")
		writeExitStatus(ch, 127)
		return
	}
	defer ptmx.Close()

	// 窗口尺寸变化 -> pty
	go func() {
		for size := range winch {
			pty.Setsize(ptmx, &pty.Winsize{Cols: uint16(size[0]), Rows: uint16(size[1])})
		}
	}()

	// client -> pty
	go func() {
		io.Copy(ptmx, ch)
		// 客户端关闭输入时发送 EOF(^D)
		ptmx.Write([]byte{4})
	}()

	// pty -> client
	io.Copy(ch, ptmx)

	err = cmd.Wait()
	writeExitStatus(ch, exitCodeOf(err))
}
