package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
)

// version is the build version string.
const version = "1.0.0"

func main() {
	var (
		addr       string
		port       int
		user       string
		pass       string
		authKeys   string
		dataDir    string
		genOnly    bool
		showVer    bool
	)

	flag.StringVar(&addr, "addr", "127.0.0.1", "监听地址（默认 127.0.0.1，设为 0.0.0.0 监听所有网卡）")
	flag.IntVar(&port, "port", 2222, "监听端口（默认 2222）")
	flag.StringVar(&user, "user", "", "登录用户名（必填）")
	flag.StringVar(&pass, "pass", "", "登录密码（可选，填写后支持密码登录）")
	flag.StringVar(&authKeys, "authkeys", "", "authorized_keys 文件路径（可选，与 -pass 至少填一个）")
	flag.StringVar(&dataDir, "data", "", "数据目录（默认可执行文件所在目录/data）")
	flag.BoolVar(&genOnly, "gen-hostkey", false, "仅生成 host 公私钥后退出")
	flag.BoolVar(&showVer, "version", false, "显示版本号")
	flag.Parse()

	if showVer {
		fmt.Printf("onessh %s\n", version)
		return
	}

	// 解析数据目录：默认可执行文件同级 data
	if dataDir == "" {
		exe, err := os.Executable()
		if err != nil {
			log.Fatalf("获取可执行文件路径失败: %v", err)
		}
		dataDir = joinPath(dirOf(exe), "data")
	}

	// 生成 / 加载 host key（首次运行自动生成）
	hostSigner, keyPath, err := loadOrCreateHostKey(dataDir)
	if err != nil {
		log.Fatalf("host 密钥初始化失败: %v", err)
	}
	fmt.Printf("[*] host 密钥: %s\n", keyPath)

	if genOnly {
		fmt.Println("[*] 已生成 host 密钥，退出。")
		return
	}

	if strings.TrimSpace(user) == "" {
		log.Fatalf("必须通过 -user 指定登录用户名")
	}
	if pass == "" && authKeys == "" {
		log.Fatalf("密码(-pass) 与 authorized_keys(-authkeys) 至少需要提供一个")
	}

	// 校验 authorized_keys 文件可读
	var keySet *authorizedKeys
	if authKeys != "" {
		keySet, err = loadAuthorizedKeys(authKeys)
		if err != nil {
			log.Fatalf("读取 authorized_keys 失败: %v", err)
		}
		if keySet.Count() == 0 {
			fmt.Fprintf(os.Stderr, "[!] 警告: authorized_keys 中没有有效的公钥: %s\n", authKeys)
		}
	}

	cfg := &serverConfig{
		user:     user,
		password: pass,
		authKeys: keySet,
	}

	hostPort := fmt.Sprintf("%s:%d", addr, port)
	srv := newServer(cfg, hostSigner, dataDir)

	fmt.Printf("[*] onessh %s\n", version)
	fmt.Printf("[*] 监听: %s\n", hostPort)
	fmt.Printf("[*] 用户: %s\n", user)
	fmt.Printf("[*] 认证: 密码=%v 公钥=%v\n", pass != "", keySet != nil && keySet.Count() > 0)
	fmt.Printf("[*] 数据目录: %s\n", dataDir)

	if err := srv.listenAndServe(hostPort); err != nil {
		log.Fatalf("服务退出: %v", err)
	}
}
