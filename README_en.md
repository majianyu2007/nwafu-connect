# NWAFU Connect

NWAFU Connect is an unofficial command-line aTrust client for Northwest A&F University's `vpn.nwafu.edu.cn` gateway. It authenticates to the VPN, consumes server-issued resource rules, and exposes SOCKS5, HTTP, DNS, port-forwarding, or TUN access.

> This project is not affiliated with Northwest A&F University or Sangfor. Follow university network and account policies. Never commit credentials, TOTP secrets, `config.toml`, or `client_data.json`.

## Authentication support

Inspect the live gateway with `nwafu-connect -auth-info`:

| Method | aTrust identifiers | Status |
| --- | --- | --- |
| LDAP account, password, and TOTP | `auth/psw` / `LDAP` | Verified with a real account |
| Phone and SMS code | `auth/smsCheckCode` / `sms73926` | Supported by the inherited aTrust SMS flow |
| WeCom | `auth/qywechat` / `wechat` | Offered by the gateway; QR login is not implemented by this CLI |

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
