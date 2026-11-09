//go:build ignore

// 开发期密钥生成工具，不参与主程序构建。
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"

	"golang.org/x/crypto/ssh"
)

func main() {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(priv)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	os.WriteFile("clientkey2", pemBytes, 0600)
	sshPub, _ := ssh.NewPublicKey(pub)
	os.WriteFile("authorized_keys", ssh.MarshalAuthorizedKey(sshPub), 0644)
}
