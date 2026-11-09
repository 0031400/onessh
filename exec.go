package main

import (
	"encoding/binary"
	"io"
	"os/exec"
	"runtime"

	"golang.org/x/crypto/ssh"
)

// parsePtyReq decodes a pty-req payload: term(string), cols, rows, width, height.
func parsePtyReq(payload []byte) (term string, cols, rows int) {
	term = "xterm"
	cols, rows = 80, 24
	if len(payload) < 4 {
		return
	}
	rest := payload[4:] // skip request type name
	if v, r, ok := readString(rest); ok {
		term = v
		rest = r
	} else {
		return
	}
	if len(rest) >= 16 {
		cols = int(binary.BigEndian.Uint32(rest[0:4]))
		rows = int(binary.BigEndian.Uint32(rest[4:8]))
		// rest[8:12] = width px, rest[12:16] = height px (ignored)
		if cols <= 0 {
			cols = 80
		}
		if rows <= 0 {
			rows = 24
		}
	}
	return
}

// parseWindowChange decodes a window-change payload: cols, rows, width, height.
func parseWindowChange(payload []byte) (cols, rows int) {
	cols, rows = 80, 24
	if len(payload) >= 8 {
		cols = int(binary.BigEndian.Uint32(payload[0:4]))
		rows = int(binary.BigEndian.Uint32(payload[4:8]))
		if cols <= 0 {
			cols = 80
		}
		if rows <= 0 {
			rows = 24
		}
	}
	return
}

// readString reads an SSH string (uint32 length + bytes).
func readString(b []byte) (string, []byte, bool) {
	if len(b) < 4 {
		return "", b, false
	}
	n := int(binary.BigEndian.Uint32(b[:4]))
	if len(b) < 4+n {
		return "", b, false
	}
	return string(b[4 : 4+n]), b[4+n:], true
}

// buildCommand constructs an exec.Cmd for the current command string,
// running as the same OS user that launched this server.
func buildCommand(cmdStr string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd.exe", "/c", cmdStr)
	}
	return exec.Command("/bin/sh", "-c", cmdStr)
}

// runExec runs a single command, wiring stdio and returning exit status.
func (s *server) runExec(ch ssh.Channel, cmdStr string, isPty bool, term string, cols, rows int) {
	cmd := buildCommand(cmdStr)
	cmd.Env = osEnviron()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeExitStatus(ch, 1)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		writeExitStatus(ch, 1)
		return
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		writeExitStatus(ch, 1)
		return
	}

	if err := cmd.Start(); err != nil {
		io.WriteString(ch.Stderr(), err.Error()+"\n")
		writeExitStatus(ch, 127)
		return
	}

	// stdin -> process
	go func() {
		io.Copy(stdin, ch)
		stdin.Close()
	}()

	done := make(chan struct{}, 2)
	go func() { io.Copy(ch, stdout); done <- struct{}{} }()
	go func() { io.Copy(ch.Stderr(), stderr); done <- struct{}{} }()
	<-done
	<-done

	err = cmd.Wait()
	writeExitStatus(ch, exitCodeOf(err))
}

// runShell starts an interactive shell with a PTY (or plain pipes if no PTY).
func (s *server) runShell(ch ssh.Channel, isPty bool, term string, cols, rows int, winch <-chan [2]int) {
	shellCmd := defaultShell()
	cmd := exec.Command(shellCmd)
	cmd.Env = osEnviron()

	if isPty && runtime.GOOS != "windows" {
		runWithPty(cmd, ch, term, cols, rows, winch)
		return
	}
	runWithPipes(cmd, ch)
}

// runWithPipes runs a shell over plain pipes (no PTY): used on Windows or when
// no pty was requested.
func runWithPipes(cmd *exec.Cmd, ch ssh.Channel) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		writeExitStatus(ch, 1)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		writeExitStatus(ch, 1)
		return
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		writeExitStatus(ch, 1)
		return
	}
	if err := cmd.Start(); err != nil {
		io.WriteString(ch.Stderr(), err.Error()+"\n")
		writeExitStatus(ch, 127)
		return
	}
	go func() { io.Copy(stdin, ch); stdin.Close() }()
	done := make(chan struct{}, 2)
	go func() { io.Copy(ch, stdout); done <- struct{}{} }()
	go func() { io.Copy(ch.Stderr(), stderr); done <- struct{}{} }()
	<-done
	<-done
	err = cmd.Wait()
	writeExitStatus(ch, exitCodeOf(err))
}

// exitCodeOf extracts an exit code from an exec error.
func exitCodeOf(err error) uint32 {
	if err == nil {
		return 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return uint32(ee.ExitCode())
	}
	return 1
}

// writeExitStatus sends the SSH "exit-status" request.
func writeExitStatus(ch ssh.Channel, code uint32) {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, code)
	ch.SendRequest("exit-status", false, payload)
}
