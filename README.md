# NWAFU Connect

NWAFU Connect 是面向西北农林科技大学 `vpn.nwafu.edu.cn` 的第三方 aTrust 命令行客户端。它登录学校 VPN、读取服务端下发的资源规则，并提供 SOCKS5、HTTP、DNS、端口转发或 TUN 接入。

> 本项目与西北农林科技大学、深信服无隶属关系。请遵守学校网络和账户管理规定。不要提交账号、密码、TOTP 密钥、`config.toml` 或 `client_data.json`。

## 支持状态

网关当前公布的认证方式可通过 `nwafu-connect -auth-info` 查看：

| 认证方式 | aTrust 标识 | 状态 |
| --- | --- | --- |
| LDAP（学号、密码、TOTP） | `auth/psw` / `LDAP` | 已完成真实账号验证 |
| 手机号、短信验证码 | `auth/smsCheckCode` / `sms73926` | 已实现交互流程，待真实号码验证 |
| 企业微信 | `auth/qywechat` / `wechat` | 已完成真实账号全流程验证（WebUI / CLI / PNG） |

本仓库只支持 aTrust。学校已经停用的 EasyConnect 实现及配置已删除。

## 快速开始

需要 Go 1.25.6。先创建本地配置：

```bash
git clone git@github.com:majianyu2007/nwafu-connect.git
cd nwafu-connect
```

```bash
cp config.toml.example config.toml
```

### LDAP + TOTP

最小配置：

```toml
server_address = "vpn.nwafu.edu.cn"
server_port = 443
username = "你的学号"
password = "你的密码"
totp_secret = "Base32 格式的验证器密钥"
auth_type = "auth/psw"
login_domain = "LDAP"
client_data_file = "client_data.json"
socks_bind = "127.0.0.1:1080"
http_bind = "127.0.0.1:1081"
```

`totp_secret` 是 Base32 格式的验证器密钥。程序会在 LDAP 密码认证成功后生成当前动态口令，并提交到 aTrust `/passport/v1/auth/token`。

### 手机号 + 短信验证码

最小配置：

```toml
server_address = "vpn.nwafu.edu.cn"
server_port = 443
auth_type = "auth/smsCheckCode"
login_domain = "sms73926"
phone = "86-你的手机号"
graph_code_file = ""
client_data_file = ""
socks_bind = ""
http_bind = ""
disable_remote_dns = true
```

`phone` 使用“国家代码-手机号”格式，例如 `86-13800138000`。运行 `go run . -config config.toml` 后，客户端先调用 `/passport/v1/public/sendSms`；如果网关要求图形验证码，会打开仅监听 `127.0.0.1` 的交互页面。收到短信后，在终端的 `Please enter the SMS verification code:` 提示处输入验证码。出现 `VPN client started` 才表示短信、ticket 换取和资源获取全链路成功。测试时不要启用 `debug_dump`，也不要频繁触发短信发送。

### 企业微信扫码登录（WebUI / CLI / PNG）

企业微信登录无需填写账号和密码，已使用真实账号完成从扫码、ticket 换取、`authCheck`、资源获取到 VPN 启动的全流程验证。

```toml
server_address = "vpn.nwafu.edu.cn"
server_port = 443
auth_type = "auth/qywechat"
login_domain = "wechat"

# 三种展示方式默认同时启用，也可独立关闭
qywechat_qrcode_browser = true
qywechat_qrcode_terminal = true
qywechat_qrcode_file = "qywechat_qrcode.png"

client_data_file = "client_data.json"
socks_bind = "127.0.0.1:1080"
http_bind = "127.0.0.1:1081"
```

二维码支持三种同时或独立使用的输出：

- **WebUI**：打开仅监听 `127.0.0.1` 的临时页面，二维码保持居中，并实时显示“等待扫码”“已扫码，等待确认”“正在完成 VPN 认证”“认证成功”或失败状态。
- **CLI**：使用紧凑的 ANSI 半块字符渲染可直接扫描的二维码，适合无桌面环境。
- **PNG 文件**：将企业微信返回的原始二维码保存到 `qywechat_qrcode_file`，文件权限固定为 `0600`；默认文件名已加入 `.gitignore`。

将 `qywechat_qrcode_browser` 或 `qywechat_qrcode_terminal` 设为 `false` 可关闭对应输出；将 `qywechat_qrcode_file` 设为空字符串可禁用文件保存。至少需要启用一种输出方式。

登录流程：

1. 从 `/passport/v1/public/authConfig` 读取网关动态下发的 `appid`、`agentid`、`redirect_uri`、`state` 和二维码超时。
2. 创建企业微信扫码会话，解析会话 key，并下载官方 PNG。
3. 按配置展示 WebUI、CLI 和文件输出。
4. 轮询企业微信的 `QRCODE_SCAN_NEVER`、`QRCODE_SCAN_ING`、`QRCODE_SCAN_FAIL`、`QRCODE_SCAN_SUCC` 状态。
5. 扫码确认后校验回调的 HTTPS 主机、端口、路径、登录域和 `state`，再向 `/passport/v1/auth/qywechat` 换取 portal ticket。
6. 解析 NWAFU 实际返回的 `/portal/qrcode_middle.html` ticket，继续 `authCheck`、资源/节点获取和 VPN 启动。

二维码默认 60 秒失效。一次真实验证的关键日志如下：

```text
Enterprise WeChat QR code scanned; waiting for confirmation
Perform GET /passport/v1/auth/qywechat
Perform GET /passport/v1/auth/authCheck
VPN client started
HTTP server listening on :1081
SOCKS5 server listening on :1080
```

运行：

```bash
go run . -config config.toml
# 或
go build -o nwafu-connect .
./nwafu-connect -config config.toml
```

默认代理：

- SOCKS5：`127.0.0.1:1080`
- HTTP：`127.0.0.1:1081`

验证 SOCKS5：

```bash
curl --socks5-hostname 127.0.0.1:1080 https://vpn.nwafu.edu.cn/portal/
```

## 常用命令

```bash
./nwafu-connect -auth-info                 # 查询网关认证方式
./nwafu-connect -version                   # 显示版本
./nwafu-connect -config config.toml        # 使用 TOML 配置启动
go test ./...                              # 单元测试
go vet ./...                               # 静态检查
go build .                                 # 本地构建
```

完整参数使用 `./nwafu-connect -h` 查看。关键选项包括：

- `-tcp-tunnel-mode`：仅使用 aTrust TCP 隧道，不支持 UDP。
- `-tun-mode -add-route -dns-hijack -fake-ip`：实验性系统 TUN 模式；通常需要管理员权限。
- `-remote-dns-server auto`：使用服务端提供的 DNS；NWAFU 当前可能不下发 DNS，程序会自动回退。
- `-client-data-file client_data.json`：保存 aTrust 会话和设备信息，减少重复登录。
- `-debug-dump`：可能记录敏感协议数据，只用于受控调试。

## 安全注意事项

- `config.toml` 和 `client_data.json` 已被 `.gitignore` 忽略；仍应限制为当前用户可读，例如 `chmod 600`。
- `client_data.json` 包含会话 Cookie 和设备标识，不应分享。
- 建议代理只监听 `127.0.0.1`；监听 `0.0.0.0` 前必须配置访问控制。
- 生产服务优先使用 `-config`，不要把密码直接写进命令行或 plist/systemd 参数。

Docker 使用见 [`docs/docker.md`](docs/docker.md)，系统服务配置见 [`docs/service.md`](docs/service.md)。
