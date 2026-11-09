//go:build ignore

// 该文件是开发期测试客户端，不参与主程序构建。
// 用法: go run -tags ignore ./testclient <addr> <user> pass|key <secret>
package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func main() {
	addr := os.Args[1]
	user := os.Args[2]
	mode := os.Args[3] // pass | key
	secret := os.Args[4]

	var auth ssh.AuthMethod
	if mode == "pass" {
		auth = ssh.Password(secret)
	} else {
		data, err := os.ReadFile(secret)
		if err != nil {
			panic(err)
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			panic(err)
		}
		auth = ssh.PublicKeys(signer)
	}
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	c, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		fmt.Println("DIAL_FAIL:", err)
		os.Exit(1)
	}
	defer c.Close()
	fmt.Println("AUTH_OK")

	// exec
	sess, err := c.NewSession()
	if err != nil {
		panic(err)
	}
	out, err := sess.CombinedOutput("echo HELLO_$((1+2)); pwd")
	fmt.Printf("EXEC_OUT=%q ERR=%v\n", string(out), err)
	sess.Close()

	// interactive shell via PTY (pty + command, collect output)
	shellSess, err := c.NewSession()
	if err == nil {
		modes := ssh.TerminalModes{ssh.ECHO: 1}
		shellSess.RequestPty("xterm", 24, 80, modes)
		outP, errP := shellSess.Output("echo SHELL_MARKER_$((40+2)); hostname")
		_ = errP
		fmt.Printf("SHELL_OUT=%q\n", string(outP))
		shellSess.Close()
	}

	// exec failing command -> exit status
	sess2, _ := c.NewSession()
	e2 := sess2.Run("exit 7")
	if ee, ok := e2.(*ssh.ExitError); ok {
		fmt.Println("EXIT_STATUS=", ee.ExitStatus())
	} else {
		fmt.Println("EXIT_STATUS err=", e2)
	}
	sess2.Close()

	// sftp
	sc, err := sftp.NewClient(c)
	if err != nil {
		fmt.Println("SFTP_FAIL:", err)
		os.Exit(1)
	}
	defer sc.Close()
	wd, _ := sc.Getwd()
	fmt.Println("SFTP_WD=", wd)
	f, err := sc.Create("sftp_test.txt")
	if err != nil {
		fmt.Println("SFTP_CREATE_FAIL:", err)
	} else {
		io.WriteString(f, "sftp-roundtrip")
		f.Close()
		rf, _ := sc.Open("sftp_test.txt")
		b, _ := io.ReadAll(rf)
		rf.Close()
		fmt.Printf("SFTP_READ=%q\n", string(b))
		sc.Remove("sftp_test.txt")
		fmt.Println("SFTP_OK")
	}
}
