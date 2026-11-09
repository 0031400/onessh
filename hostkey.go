package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
)

// joinPath / dirOf are small wrappers kept for clarity (platform separators).
func joinPath(a, b string) string { return filepath.Join(a, b) }
func dirOf(p string) string      { return filepath.Dir(p) }

// ensureDir creates the directory (recursively) if it does not exist.
func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0o700)
}

// hostKeyPaths returns the private/public host key file paths inside dataDir.
func hostKeyPaths(dataDir string) (priv, pub string) {
	return filepath.Join(dataDir, "host_ed25519"),
		filepath.Join(dataDir, "host_ed25519.pub")
}

// loadOrCreateHostKey loads the ed25519 host key from dataDir, generating it
// on first run. It returns the ssh.Signer and the private key path.
func loadOrCreateHostKey(dataDir string) (ssh.Signer, string, error) {
	if err := ensureDir(dataDir); err != nil {
		return nil, "", fmt.Errorf("创建数据目录失败: %w", err)
	}
	privPath, pubPath := hostKeyPaths(dataDir)

	if _, err := os.Stat(privPath); err == nil {
		data, err := os.ReadFile(privPath)
		if err != nil {
			return nil, privPath, fmt.Errorf("读取 host 私钥失败: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(data)
		if err != nil {
			return nil, privPath, fmt.Errorf("解析 host 私钥失败: %w", err)
		}
		return signer, privPath, nil
	} else if !os.IsNotExist(err) {
		return nil, privPath, fmt.Errorf("检查 host 私钥失败: %w", err)
	}

	// 首次运行：生成 ed25519 host key
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, privPath, fmt.Errorf("生成 ed25519 密钥失败: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, privPath, fmt.Errorf("编码私钥失败: %w", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(privPath, privPEM, 0o600); err != nil {
		return nil, privPath, fmt.Errorf("写入 host 私钥失败: %w", err)
	}

	// 写公钥（OpenSSH 格式）
	pubKey, err := ssh.NewPublicKey(priv.Public())
	if err != nil {
		return nil, privPath, fmt.Errorf("生成公钥失败: %w", err)
	}
	auth := ssh.MarshalAuthorizedKey(pubKey)
	if err := os.WriteFile(pubPath, auth, 0o644); err != nil {
		return nil, privPath, fmt.Errorf("写入 host 公钥失败: %w", err)
	}

	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, privPath, fmt.Errorf("创建 host signer 失败: %w", err)
	}
	return signer, privPath, nil
}
