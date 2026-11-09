package main

import (
	"bytes"
	"crypto/subtle"
	"fmt"
	"os"

	"golang.org/x/crypto/ssh"
)

// authorizedKeys holds parsed public keys from an authorized_keys file.
type authorizedKeys struct {
	keys []ssh.PublicKey
}

// Count returns the number of parsed keys.
func (a *authorizedKeys) Count() int {
	if a == nil {
		return 0
	}
	return len(a.keys)
}

// loadAuthorizedKeys reads and parses an authorized_keys file.
// Blank lines and #-comments are skipped, as per standard format.
func loadAuthorizedKeys(path string) (*authorizedKeys, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	set := &authorizedKeys{}
	for _, line := range bytes.Split(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		pk, _, _, _, err := ssh.ParseAuthorizedKey(line)
		if err != nil {
			// 跳过无法解析的行（注释、选项前缀等），记录但不中断
			fmt.Fprintf(os.Stderr, "[!] 跳过无法解析的 authorized_keys 行: %v\n", err)
			continue
		}
		set.keys = append(set.keys, pk)
	}
	return set, nil
}

// Match reports whether pk matches one of the loaded keys.
func (a *authorizedKeys) Match(pk ssh.PublicKey) bool {
	if a == nil {
		return false
	}
	for _, k := range a.keys {
		if bytes.Equal(k.Marshal(), pk.Marshal()) {
			return true
		}
	}
	return false
}

// constantTimeEqual compares two strings in constant time.
func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		// 仍做一次比较，避免明显的时间差
		subtle.ConstantTimeCompare([]byte(a), []byte(a))
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
