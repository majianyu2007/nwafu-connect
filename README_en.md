# NWAFU Connect

> **Disclaimer**
>
> This software is provided **as is** and is not affiliated with Northwest A&F University or Sangfor. No warranty is made regarding correctness or reliability. Follow university network and account policies; you are solely responsible for any consequences of using this tool. **Never commit or share credentials, TOTP secrets, `config.toml`, or `client_data.json`.**

---

[中文](README.md) | English

NWAFU Connect is an unofficial **aTrust** client for Northwest A&F University's `vpn.nwafu.edu.cn` gateway, shipping as a desktop tray GUI, cross-platform CLI, and embeddable Go/C/mobile core library. Built upon upstream [zju-connect](https://github.com/Mythologyli/zju-connect) and [EasierConnect](https://github.com/lyc8503/EasierConnect), all obsolete EasyConnect code has been removed in favor of NWAFU's aTrust authentication, isolated managed browser mode, SOCKS5/HTTP/Shadowsocks/DNS/port-forwarding proxies, and OS TUN mode.

> **Campus Network Notice**
>
> **`vpn.nwafu.edu.cn` is not directly reachable from inside the NWAFU campus network**, where campus services are already directly accessible without a VPN. Use this client from off-campus networks only; the desktop tray automatically detects unreachable gateway states and avoids reconnect loops.

## Downloads & Multi-Platform Matrix

Every tagged release (`v*`) publishes a unified set of assets plus `SHA256SUMS.txt` via GitHub Actions:

| Target | Packages | Architectures | Contents |
| --- | --- | --- | --- |
| **Windows Desktop** | Installer (`-setup.exe`) & Portable (`-portable.zip`) | `amd64`, `arm64` | Tray app, settings window, managed browser, `nwafu-connect`, `nwafu-connect-proxy` |
| **macOS Desktop** | Disk Image (`.dmg`) & Portable (`-portable.zip`) | `arm64` (Apple Silicon), `amd64` (Intel) | Native `.app` bundle (see quarantine removal note below) |
| **Linux Desktop** | Package (`.deb`) & Portable (`-portable.tar.gz`) | `amd64`, `arm64` | Tray app, desktop launcher, managed browser |
| **Cross-Platform CLI** | `nwafu-connect-<os>-<arch>.zip` (17 targets) | Windows (`amd64/386/arm64`), macOS (`amd64/arm64`), Linux (`amd64/386/arm64/armv5/armv6/armv7/riscv64/mips/mipsle/mips64/mips64le`) | Pure-Go (`CGO_ENABLED=0`) `nwafu-connect`, `nwafu-connect-proxy`, `config.toml.example` |
| **Android Core Libs** | `-android-jnilibs.zip` (C ABI `.so`) & `-android-aar.zip` (`gomobile` `VpnService` `.aar`) | `arm64-v8a`, `armeabi-v7a`, `x86_64` | Ready for Android / Flutter / Kotlin host apps (proxy or TUN VPN) |
| **Apple Core Libs** | `-ios-xcframework.zip` & `-macos-universal-lib.zip` | iOS device/sim `arm64`, macOS universal (`arm64`+`amd64`) | Static `NwafuConnect.xcframework` and universal `.dylib` + C header |
| **Docker Image** | Multi-arch Distroless image | `linux/amd64`, `linux/arm64` | See [`docs/docker_en.md`](docs/docker_en.md) |

> On macOS, clear the quarantine attribute before first launch of unsigned builds:
> ```bash
> sudo xattr -rd com.apple.quarantine "/Applications/NWAFU Connect.app"
> # or for CLI binaries:
> sudo xattr -rd com.apple.quarantine ./nwafu-connect ./nwafu-connect-proxy
> ```

### GUI Client Options

- **NWAFU Connect Desktop (Recommended)**: Download the installer or portable archive from [Releases](https://github.com/majianyu2007/nwafu-connect/releases). Preconfigured for `vpn.nwafu.edu.cn:443` with LDAP+TOTP, SMS, and WeCom QR login, one-click isolated managed browser launch, resource portal, local proxy copy action, and wake/network recovery.
- **Third-Party Compatible aTrust GUIs**: You can also try [EZ4Connect](https://github.com/chenx-dust/EZ4Connect) by [Chenx Dust](https://github.com/chenx-dust) (supports aTrust; set protocol to **aTrust**, server to `vpn.nwafu.edu.cn:443`, login domain `LDAP` / `sms73926` / `wechat`) or [hitsz-connect-verge](https://github.com/kowyo/hitsz-connect-verge) by [kowyo](https://github.com/kowyo).
- **Mobile Integration (Android / iOS)**: This repository publishes Android `jniLibs` (`.so`), Android `gomobile` `VpnService` `.aar`, and iOS `NwafuConnect.xcframework` plus the Go [`embed`](embed/embed.go) package and [`scripts/build-lib.sh`](scripts/build-lib.sh). Standalone Android APK and unsigned iOS IPA shells are planned as a dedicated follow-up app project.

## Authentication Support

Inspect the gateway from an off-campus network with `nwafu-connect -auth-info`:

| Method | aTrust identifiers (`auth_type` / `login_domain`) | Status |
| --- | --- | --- |
| LDAP account, password, and TOTP | `auth/psw` / `LDAP` | Verified with a real account |
| WeCom QR login (WebUI / CLI / PNG) | `auth/qywechat` / `wechat` | Full live-account flow verified |
| Phone + SMS code | `auth/smsCheckCode` / `sms73926` | Interactive flow implemented (terminal & loopback browser fallback); live phone test pending |
| CAS / HTTPS OAuth2 (generic aTrust) | `auth/cas`, `auth/httpsOauth2` | Synced from upstream with enhanced-auth support |

## Quick Start (CLI)

Download a release archive or build locally with Go 1.25.6+:

```bash
git clone https://github.com/majianyu2007/nwafu-connect.git
cd nwafu-connect
cp config.toml.example config.toml
chmod 600 config.toml
```

### LDAP + TOTP

```toml
server_address = "vpn.nwafu.edu.cn"
server_port = 443
username = "student_id"
password = "password"
totp_secret = "Base32 authenticator secret"
auth_type = "auth/psw"
login_domain = "LDAP"
client_data_file = "client_data.json"
socks_bind = "127.0.0.1:1080"
http_bind = "127.0.0.1:1081"
```

Or pass sensitive fields via environment variables:

```bash
NWAFU_CONNECT_PASSWORD="password" NWAFU_CONNECT_TOTP_SECRET="secret" \
  ./nwafu-connect --config config.toml
```

### WeCom QR Login (WebUI / CLI / PNG)

```toml
server_address = "vpn.nwafu.edu.cn"
server_port = 443
auth_type = "auth/qywechat"
login_domain = "wechat"
qywechat_qrcode_browser = true
qywechat_qrcode_terminal = true
qywechat_qrcode_file = "qywechat_qrcode.png"
client_data_file = "client_data.json"
```

### Managed Browser Mode

```toml
browser_mode = true
browser_url = "" # Empty displays the application-grouped campus resource portal
browser_path = "" # Auto-detect Chrome, Edge, Chromium, or Brave
```

Managed browser mode spawns a private random-port loopback HTTP proxy and an isolated Chromium profile without modifying system proxies or user profiles. Only gateway-authorized IP/domain destinations are permitted (strict VPN-only ACL), and encrypted DoH is used when the gateway supplies no DNS server to avoid Fake-IP contamination.

### TUN Mode Notes

1. Administrator / `root` privileges are required.
2. On Windows, download the matching architecture `wintun.dll` from [wintun.net](https://www.wintun.net) and place it next to `nwafu-connect.exe`.
3. Recommended aTrust TUN flags:
   ```bash
   sudo ./nwafu-connect --config config.toml --tun-mode --add-route --dns-hijack --fake-ip
   ```
4. If another TUN + Fake-IP proxy tool is active simultaneously, configure split rules carefully and avoid enabling `auto_detect_interface`.

## Configuration Precedence & Key Flags

Configuration merges in order: **built-in defaults < TOML file < `NWAFU_CONNECT_*` environment variables < explicit CLI flags**. Legacy single-dash flags (`-config`, `-auth-info`, `-tun-mode`) remain fully supported.

Every key maps to an uppercase `NWAFU_CONNECT_` environment variable (e.g. `NWAFU_CONNECT_CONFIG`, `NWAFU_CONNECT_PASSWORD`, `NWAFU_CONNECT_SESSION_REFRESH_INTERVAL`, `NWAFU_CONNECT_TCP_PORT_FORWARDING`, `NWAFU_CONNECT_CUSTOM_DNS`).

Key flags (run `./nwafu-connect -h` for the complete list):

- `--session-refresh-interval`: aTrust SID refresh interval in seconds (default `1800`, `0` disables).
- `--update-best-nodes-interval`: Best-node probe interval in seconds (default `300`, `0` disables).
- `--secondary-dns-server`: Fallback DNS (`auto` prefers secondary policy DNS or `114.114.114.114`; empty uses host DNS).
- `--local-dns-server`: DNS server (`IP` or `IP:port`) used to resolve the VPN gateway hostname itself.
- `--bind-interface` / `--auto-detect-interface`: Bind VPN underlay sockets to a specific interface or auto-detect it.
- `--proxy-all`: Route all destinations through the VPN tunnel.
- `--trust-device` / `--untrust-device`: Trust or untrust the current device using `--client-data-file`.
- `--debug-pcap-file` / `--debug-tls-log-file`: Export reconstructed underlay PCAP and NSS TLS key log for controlled debugging.

## SSH / SFTP Client Access

Use the bundled `nwafu-connect-proxy` stdio helper with OpenSSH `ProxyCommand`:

```bash
ssh -o 'ProxyCommand="/Applications/NWAFU Connect.app/Contents/MacOS/nwafu-connect-proxy" --proxy 127.0.0.1:63665 --target %h:%p' user@10.133.16.10
```

## Security & Deployment

- Keep `config.toml` and `client_data.json` mode `0600` and out of version control.
- Bind proxies to `127.0.0.1` unless remote access is explicitly secured.
- See [`docs/docker_en.md`](docs/docker_en.md) for Docker and [`docs/service_en.md`](docs/service_en.md) for system services.

## Credits

- [zju-connect](https://github.com/Mythologyli/zju-connect)
- [EasierConnect](https://github.com/lyc8503/EasierConnect)
- [EZ4Connect](https://github.com/chenx-dust/EZ4Connect)
