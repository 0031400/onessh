package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// serverConfig holds authentication configuration.
type serverConfig struct {
	user     string
	password string
	authKeys *authorizedKeys
}

// server is the SSH server.
type server struct {
	cfg      *serverConfig
	hostKey  ssh.Signer
	dataDir  string
	listener net.Listener
	mu       sync.Mutex
	conns    map[string]*ssh.ServerConn
}

func newServer(cfg *serverConfig, hostKey ssh.Signer, dataDir string) *server {
	return &server{
		cfg:     cfg,
		hostKey: hostKey,
		dataDir: dataDir,
		conns:   make(map[string]*ssh.ServerConn),
	}
}

// sshConfig builds the *ssh.ServerConfig with password and public key auth.
func (s *server) sshConfig() *ssh.ServerConfig {
	sc := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if conn.User() != s.cfg.user {
				return nil, fmt.Errorf("unknown user %q", conn.User())
			}
			if s.cfg.password == "" {
				return nil, errors.New("password auth not enabled")
			}
			if constantTimeEqual(string(password), s.cfg.password) {
				return nil, nil
			}
			return nil, errors.New("invalid password")
		},
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if conn.User() != s.cfg.user {
				return nil, fmt.Errorf("unknown user %q", conn.User())
			}
			if s.cfg.authKeys == nil || s.cfg.authKeys.Count() == 0 {
				return nil, errors.New("public key auth not enabled")
			}
			if s.cfg.authKeys.Match(key) {
				return nil, nil
			}
			return nil, errors.New("public key not authorized")
		},
	}
	sc.AddHostKey(s.hostKey)
	return sc
}

// listenAndServe starts the listener and accepts connections.
func (s *server) listenAndServe(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	s.listener = ln
	for {
		c, err := ln.Accept()
		if err != nil {
			select {
			case <-shutdownChan:
				return nil
			default:
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return err
		}
		go s.handleRawConn(c)
	}
}

// handleRawConn performs the SSH handshake then dispatches channels.
func (s *server) handleRawConn(raw net.Conn) {
	sc := s.sshConfig()
	sconn, chans, reqs, err := ssh.NewServerConn(raw, sc)
	if err != nil {
		log.Printf("[!] SSH 握手失败 (%s): %v", raw.RemoteAddr(), err)
		raw.Close()
		return
	}
	defer sconn.Close()

	addr := sconn.RemoteAddr().String()
	s.mu.Lock()
	s.conns[addr] = sconn
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, addr)
		s.mu.Unlock()
	}()
	log.Printf("[+] 已连接: user=%s from=%s", sconn.User(), addr)

	// 全局请求（如 keepalive / tcpip-forward）——默认拒绝
	go func() {
		for req := range reqs {
			switch req.Type {
			case "keepalive@openssh.com", "no-more-sessions@openssh.com":
				if req.WantReply {
					req.Reply(true, nil)
				}
			default:
				if req.WantReply {
					req.Reply(false, nil)
				}
			}
		}
	}()

	for newChan := range chans {
		switch newChan.ChannelType() {
		case "session":
			go s.handleSession(newChan)
		case "direct-tcpip":
			go s.handleDirectTCPIP(newChan)
		default:
			newChan.Reject(ssh.UnknownChannelType, "unsupported channel type")
		}
	}
	log.Printf("[-] 断开: %s", addr)
}

// handleSession handles a session channel: exec, shell, pty, sftp subsystem.
func (s *server) handleSession(newChan ssh.NewChannel) {
	ch, reqs, err := newChan.Accept()
	if err != nil {
		log.Printf("[!] 接受 session 通道失败: %v", err)
		return
	}
	defer ch.Close()

	var (
		term  = "xterm"
		cols  = 80
		rows  = 24
		isPty = false
	)
	// winch 传递窗口尺寸变化给正在运行的 PTY shell。
	winch := make(chan [2]int, 4)

	for req := range reqs {
		switch req.Type {
		case "exec":
			cmd := string(req.Payload[4:])
			if req.WantReply {
				req.Reply(true, nil)
			}
			s.runExec(ch, cmd, isPty, term, cols, rows)
			return

		case "shell":
			if req.WantReply {
				req.Reply(true, nil)
			}
			s.runShell(ch, isPty, term, cols, rows, winch)
			return

		case "pty-req":
			term, cols, rows = parsePtyReq(req.Payload)
			isPty = true
			if req.WantReply {
				req.Reply(true, nil)
			}

		case "window-change":
			c, r := parseWindowChange(req.Payload)
			select {
			case winch <- [2]int{c, r}:
			default:
			}
			if req.WantReply {
				req.Reply(true, nil)
			}

		case "env":
			if req.WantReply {
				req.Reply(true, nil)
			}

		case "subsystem":
			name := string(req.Payload[4:])
			if name == "sftp" {
				if req.WantReply {
					req.Reply(true, nil)
				}
				s.runSFTP(ch)
				return
			}
			if req.WantReply {
				req.Reply(false, nil)
			}

		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

// handleDirectTCPIP is disabled by default (no port forwarding).
func (s *server) handleDirectTCPIP(newChan ssh.NewChannel) {
	newChan.Reject(ssh.Prohibited, "port forwarding not supported")
}

// runSFTP starts an SFTP server over the channel.
func (s *server) runSFTP(ch ssh.Channel) {
	server, err := sftp.NewServer(ch)
	if err != nil {
		log.Printf("[!] 启动 SFTP 失败: %v", err)
		return
	}
	defer server.Close()
	if err := server.Serve(); err != nil && !errors.Is(err, io.EOF) {
		log.Printf("[!] SFTP 会话结束: %v", err)
	}
}

// shutdownChan is closed on shutdown to stop the accept loop.
var shutdownChan = make(chan struct{})

// osEnviron returns the current environment as "KEY=VALUE" strings.
func osEnviron() []string {
	return os.Environ()
}
