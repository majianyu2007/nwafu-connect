# NWAFU Connect

NWAFU Connect is an unofficial command-line aTrust client for Northwest A&F University's `vpn.nwafu.edu.cn` gateway. It authenticates to the VPN, consumes server-issued resource rules, and exposes SOCKS5, HTTP, DNS, port-forwarding, or TUN access.

> This project is not affiliated with Northwest A&F University or Sangfor. Follow university network and account policies. Never commit credentials, TOTP secrets, `config.toml`, or `client_data.json`.

## Authentication support

Inspect the live gateway with `nwafu-connect -auth-info`:

| Method | aTrust identifiers | Status |
| --- | --- | --- |
| LDAP account, password, and TOTP | `auth/psw` / `LDAP` | Verified with a real account |
| Phone and SMS code | `auth/smsCheckCode` / `sms73926` | Interactive flow implemented; live phone verification pending |
| WeCom | `auth/qywechat` / `wechat` | Browser-based QR login implemented |

This repository is aTrust-only. The obsolete EasyConnect implementation and configuration have been removed.

## Quick start

Go 1.25.6 is required. Create a local configuration:

```bash
git clone git@github.com:majianyu2007/nwafu-connect.git
cd nwafu-connect
```

```bash
cp config.toml.example config.toml
```

Minimal LDAP + TOTP configuration:

```toml
server_address = "vpn.nwafu.edu.cn"
server_port = 443
username = "student ID"
password = "password"
totp_secret = "Base32 authenticator secret"
auth_type = "auth/psw"
login_domain = "LDAP"
client_data_file = "client_data.json"
socks_bind = "127.0.0.1:1080"
http_bind = "127.0.0.1:1081"
```

`totp_secret` is the Base32 authenticator seed. After LDAP password authentication, the client generates the current token and submits it to aTrust `/passport/v1/auth/token`.

Minimal phone + SMS configuration:

```toml
server_address = "vpn.nwafu.edu.cn"
server_port = 443
auth_type = "auth/smsCheckCode"
login_domain = "sms73926"
phone = "86-your-phone-number"
graph_code_file = ""
client_data_file = ""
socks_bind = ""
http_bind = ""
disable_remote_dns = true
```

Use `country-code-phone-number` format for `phone`, for example `86-13800138000`. After `go run . -config config.toml`, the client first calls `/passport/v1/public/sendSms`; if the gateway requires a graphical captcha, it opens an interactive page bound only to `127.0.0.1`. Enter the received code at the terminal's `Please enter the SMS verification code:` prompt. Only `VPN client started` confirms the complete SMS, ticket exchange, and resource-loading path. Keep `debug_dump` disabled and avoid repeatedly requesting SMS codes.

WeCom QR login does not require a username or password:

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

On startup, the client opens a temporary page bound only to `127.0.0.1`, renders the QR code in the CLI, and saves the original PNG with mode `0600` in the current directory. Control these outputs independently with `qywechat_qrcode_browser`, `qywechat_qrcode_terminal`, and `qywechat_qrcode_file`. The browser page reports waiting, confirmation, success, and failure states. The client polls WeCom, validates the callback host, path, login domain, and `state`, then exchanges it for the current aTrust ticket. The QR code expires after 60 seconds by default.

Run the client:

```bash
go run . -config config.toml
# or
go build -o nwafu-connect .
./nwafu-connect -config config.toml
```

Default proxies:

- SOCKS5: `127.0.0.1:1080`
- HTTP: `127.0.0.1:1081`

Smoke-test SOCKS5:

```bash
curl --socks5-hostname 127.0.0.1:1080 https://vpn.nwafu.edu.cn/portal/
```

## Common commands

```bash
./nwafu-connect -auth-info
./nwafu-connect -version
./nwafu-connect -config config.toml
go test ./...
go vet ./...
go build .
```

Use `./nwafu-connect -h` for every flag. Important options:

- `-tcp-tunnel-mode`: aTrust TCP tunnel only; UDP is unavailable.
- `-tun-mode -add-route -dns-hijack -fake-ip`: experimental system TUN mode, normally requiring administrator privileges.
- `-remote-dns-server auto`: use the server-provided DNS. The NWAFU gateway may omit it, in which case the client falls back automatically.
- `-client-data-file client_data.json`: persist aTrust session and device state.
- `-debug-dump`: may log sensitive protocol data; use only in controlled debugging.

## Security

- `config.toml` and `client_data.json` are ignored by Git, but should still be mode `0600`.
- `client_data.json` contains session cookies and device identifiers. Do not share it.
- Bind proxies to `127.0.0.1` unless remote access is explicitly secured.
- For services, use `-config`; do not place passwords in command-line arguments, plist files, or systemd units.

See [`docs/docker_en.md`](docs/docker_en.md) for containers and [`docs/service_en.md`](docs/service_en.md) for service installation.
