# NWAFU Connect

NWAFU Connect 是面向西北农林科技大学 `vpn.nwafu.edu.cn` 的第三方 aTrust 命令行客户端。它登录学校 VPN、读取服务端下发的资源规则，并提供 SOCKS5、HTTP、DNS、端口转发或 TUN 接入。

> 本项目与西北农林科技大学、深信服无隶属关系。请遵守学校网络和账户管理规定。不要提交账号、密码、TOTP 密钥、`config.toml` 或 `client_data.json`。

## 支持状态

网关当前公布的认证方式可通过 `nwafu-connect -auth-info` 查看：

| 认证方式 | aTrust 标识 | 状态 |
| --- | --- | --- |
| LDAP（学号、密码、TOTP） | `auth/psw` / `LDAP` | 已完成真实账号验证 |
| 手机号、短信验证码 | `auth/smsCheckCode` / `sms73926` | 已实现交互流程，待真实号码验证 |
| 企业微信 | `auth/qywechat` / `wechat` | 已实现浏览器扫码登录 |

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

LDAP + TOTP 的最小配置：

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

手机号 + 短信验证码的最小配置：

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

企业微信扫码登录无需填写账号和密码：

```toml
server_address = "vpn.nwafu.edu.cn"
server_port = 443
auth_type = "auth/qywechat"
login_domain = "wechat"
qywechat_qrcode_browser = true
qywechat_qrcode_terminal = true
qywechat_qrcode_file = "qywechat_qrcode.png"
client_data_file = "client_data.json"
socks_bind = "127.0.0.1:1080"
http_bind = "127.0.0.1:1081"
```

启动后程序会打开仅监听 `127.0.0.1` 的临时页面、在 CLI 渲染二维码，并将原始 PNG 以 `0600` 权限保存到当前目录。三种输出可分别通过 `qywechat_qrcode_browser`、`qywechat_qrcode_terminal` 和 `qywechat_qrcode_file` 控制。浏览器页面会显示等待扫码、确认中、成功或失败状态。客户端轮询企业微信扫码结果，校验回调的主机、路径、登录域和 `state`，再向当前 aTrust 会话换取 ticket；二维码默认 60 秒失效。

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
