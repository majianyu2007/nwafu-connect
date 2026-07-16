# NWAFU Connect

NWAFU Connect is an unofficial command-line aTrust client for Northwest A&F University's `vpn.nwafu.edu.cn` gateway. It authenticates to the VPN, consumes server-issued resource rules, and exposes SOCKS5, HTTP, DNS, port-forwarding, or TUN access.

> This project is not affiliated with Northwest A&F University or Sangfor. Follow university network and account policies. Never commit credentials, TOTP secrets, `config.toml`, or `client_data.json`.

## Authentication support

Inspect the live gateway with `nwafu-connect -auth-info`:

| Method | aTrust identifiers | Status |
| --- | --- | --- |
| LDAP account, password, and TOTP | `auth/psw` / `LDAP` | Verified with a real account |
| Phone and SMS code | `auth/smsCheckCode` / `sms73926` | Interactive flow implemented; live phone verification pending |
| WeCom | `auth/qywechat` / `wechat` | Full live-account flow verified (WebUI / CLI / PNG) |

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

### LDAP + TOTP

Minimal configuration:

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

### Phone + SMS code

Minimal configuration:

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

### WeCom QR login (WebUI / CLI / PNG)

WeCom login requires no username or password. The complete live-account path—from scanning and ticket exchange through `authCheck`, resource loading, and VPN startup—has been verified.

```toml
server_address = "vpn.nwafu.edu.cn"
server_port = 443
auth_type = "auth/qywechat"
login_domain = "wechat"

# All three outputs are enabled by default and can be controlled independently
qywechat_qrcode_browser = true
qywechat_qrcode_terminal = true
qywechat_qrcode_file = "qywechat_qrcode.png"

client_data_file = "client_data.json"
socks_bind = "127.0.0.1:1080"
http_bind = "127.0.0.1:1081"
```

The QR code supports three simultaneous or independent outputs:

- **WebUI**: opens a temporary page bound only to `127.0.0.1`, keeps the QR code centered, and reports waiting, scanned/pending confirmation, VPN authentication, success, or failure states.
- **CLI**: renders a compact, directly scannable QR code with ANSI half-block characters for headless environments.
- **PNG file**: saves the original WeCom QR image to `qywechat_qrcode_file` with fixed mode `0600`; the default filename is listed in `.gitignore`.

Set `qywechat_qrcode_browser` or `qywechat_qrcode_terminal` to `false` to disable that output. Set `qywechat_qrcode_file` to an empty string to disable file saving. At least one output must remain enabled.

Login flow:

1. Read the gateway-provided `appid`, `agentid`, `redirect_uri`, `state`, and QR timeout from `/passport/v1/public/authConfig`.
2. Create the WeCom scan session, parse its key, and download the official PNG.
3. Present the configured WebUI, CLI, and file outputs.
4. Poll the `QRCODE_SCAN_NEVER`, `QRCODE_SCAN_ING`, `QRCODE_SCAN_FAIL`, and `QRCODE_SCAN_SUCC` states.
5. After confirmation, validate the callback HTTPS host, port, path, login domain, and `state`, then exchange it through `/passport/v1/auth/qywechat` for the portal ticket.
6. Parse the ticket from NWAFU's actual `/portal/qrcode_middle.html` redirect, then continue through `authCheck`, resource/node loading, and VPN startup.

The QR code expires after 60 seconds by default. A successful live run includes:

```text
Enterprise WeChat QR code scanned; waiting for confirmation
Perform GET /passport/v1/auth/qywechat
Perform GET /passport/v1/auth/authCheck
VPN client started
HTTP server listening on :1081
SOCKS5 server listening on :1080
```

## Managed browser mode

If the only goal is browsing campus web services, enable managed browser mode instead of configuring SOCKS5, a system proxy, TUN routes, or additional split-routing rules:

```toml
browser_mode = true
browser_url = "" # Show a home page built from this aTrust session's resources
browser_path = "" # Auto-detect Chrome, Edge, Chromium, or Brave
```

After `nwafu-connect -config config.toml` starts:

1. The client completes aTrust authentication and loads the server-issued resource rules.
2. It starts a private HTTP CONNECT proxy on a random `127.0.0.1` port.
3. It launches a Chromium-based browser with an isolated temporary profile, without changing the system proxy or touching the user's normal browser data.
4. Every browser HTTP/HTTPS request enters the private proxy. Only IP/domain resources authorized by the aTrust gateway enter the VPN; unauthorized destinations are rejected locally and never fall back to a host direct connection or host proxy. When the gateway provides no DNS server, browser mode uses encrypted DoH without the system resolver to prevent Clash/FlClash Fake-IP contamination. QUIC and unproxied WebRTC UDP are disabled to prevent bypasses.
5. Closing the last managed browser window removes the temporary profile and shuts down the proxy, resolver, tunnel, and aTrust session in order.

Browser mode does not start configured public SOCKS5, HTTP, DNS, Shadowsocks, or port-forwarding listeners. Its default home page groups the current aTrust session's complete resource metadata by application, including name, description, every address, protocol, and port range. `browser_url` only overrides the optional start page and is not an authorization or routing rule.

The lightweight implementation currently supports Windows, macOS, and Linux by reusing an installed Chrome, Edge, Chromium, or Brave executable rather than adding hundreds of megabytes of Chromium to every release. Windows normally has Edge available; on other systems, set `browser_path` if auto-detection finds no browser. Android and iOS require separate application shells and platform WebViews and are not part of this desktop implementation.

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
