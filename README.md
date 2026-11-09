# onessh

一个用 Go 编写的轻量 SSH 服务端，支持 Windows / Linux，提供 **远程命令执行、交互式 Shell（PTY）、SFTP** 三大能力。

- 单文件可执行程序，无外部依赖
- 支持 **密码登录** 和 **公钥登录**（`authorized_keys`）两种认证，可同时启用
- **首次运行自动生成 host 密钥**（ed25519），所有数据放在 *可执行文件同级 `data/` 目录*
- 执行命令时以 **启动本程序的系统用户** 身份运行（不做权限切换）
- 监听地址与端口可配置，默认仅监听本地 `127.0.0.1:2222`

## 编译

需要 Go 1.21+。

```bash
# 当前平台
go build -o onessh .

# 交叉编译 Linux amd64
GOOS=linux GOARCH=amd64 go build -o onessh_linux_amd64 .

# 交叉编译 Linux arm64
GOOS=linux GOARCH=arm64 go build -o onessh_linux_arm64 .
```

> Windows 上交叉编译示例（PowerShell）：
> ```powershell
> $env:GOOS="linux"; $env:GOARCH="amd64"; go build -o onessh_linux_amd64 .
> ```

## 命令行参数

| 参数 | 说明 | 必填 |
| --- | --- | --- |
| `-addr` | 监听地址，默认 `127.0.0.1`（设为 `0.0.0.0` 监听所有网卡） | 否 |
| `-port` | 监听端口，默认 `2222` | 否 |
| `-user` | 登录用户名 | **是** |
| `-pass` | 登录密码（提供即启用密码登录） | 否* |
| `-authkeys` | authorized_keys 文件路径（提供即启用公钥登录） | 否* |
| `-data` | 数据目录，默认"可执行文件所在目录/data" | 否 |
| `-gen-hostkey` | 仅生成 host 公私钥后退出 | 否 |
| `-version` | 显示版本 | 否 |

\* `-pass` 与 `-authkeys` **至少提供一个**，否则启动报错退出。

## 使用示例

### 仅密码登录
```bash
./onessh -user alice -pass "secret" -addr 0.0.0.0 -port 2222
ssh alice@<server-ip> -p 2222
```

### 仅公钥登录
```bash
# authorized_keys 为标准 OpenSSH 格式，一行一个公钥
./onessh -user alice -authkeys /home/alice/.ssh/authorized_keys -addr 0.0.0.0
ssh -i ~/.ssh/id_ed25519 alice@<server-ip>
```

### 同时启用密码 + 公钥
```bash
./onessh -user alice -pass "secret" -authkeys ./authorized_keys
```

### 生成 host 密钥（首次运行会自动做，也可手动）
```bash
./onessh -gen-hostkey
```

## 数据目录

首次运行会在 **可执行文件同级** 创建 `data/` 目录并生成：

```
data/
├── host_ed25519       # host 私钥（PEM，权限 0600）
└── host_ed25519.pub   # host 公钥（OpenSSH 格式）
```

`data/` 内含私钥，**请妥善保管，切勿提交到版本库**（已在 `.gitignore` 中忽略）。

## 功能说明

- **命令执行（exec）**：`ssh user@host "命令"`，通过系统 shell 执行（Linux：`/bin/sh -c`；Windows：`cmd.exe /c`），返回真实退出码。
- **交互式 Shell（PTY）**：`ssh user@host`（Linux 下分配 PTY，支持窗口大小与 resize；Windows 下为管道模式）。
- **SFTP**：`sftp -P 2222 user@host`，或任意 SFTP 客户端（支持 `github.com/pkg/sftp` 兼容的完整文件操作）。
- **权限模型**：没有用户切换，所有命令以启动服务的系统账户身份执行。请以合适的低权限账户运行本程序。

## 目录结构

```
onessh/
├── main.go          # 入口、参数解析
├── hostkey.go       # host 密钥生成/加载、data 目录
├── auth.go          # 密码/公钥认证、authorized_keys 解析
├── server.go        # SSH 服务、会话分发
├── exec.go          # 命令执行、PTY 请求解析
├── pty_unix.go      # Linux/macOS PTY 实现
├── pty_windows.go   # Windows 下的管道回退
├── go.mod / go.sum
└── testclient/      # 开发期测试客户端（build tag: ignore，不参与构建）
```
