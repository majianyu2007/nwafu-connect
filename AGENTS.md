# Repository Guidelines

## Project Overview

NWAFU Connect is a Go aTrust client specialized for Northwest A&F University's `vpn.nwafu.edu.cn` gateway. It authenticates with LDAP/password/TOTP, WeCom QR, or SMS (plus upstream CAS/OAuth2 support), consumes server-issued resource rules, and exposes managed browser access, SOCKS5, HTTP, DNS, Shadowsocks, port forwarding, TCP-tunnel, or TUN access, alongside a Fyne desktop tray GUI (`cmd/nwafu-connect-desktop`), embeddable Go/C API (`embed/`, `cmd/libnwafuconnect`), and Android `VpnService` bindings (`mobile/`). The Go module and canonical repository are `github.com/majianyu2007/nwafu-connect`.

> **Campus network note**: `vpn.nwafu.edu.cn` is not directly reachable from inside the NWAFU campus network. Do not assume live gateway connectivity during local development or automated tests.

## Architecture & Data Flow

1. `init.go` merges defaults, TOML (`koanf`), `NWAFU_CONNECT_*` environment variables, and explicit `pflag` CLI flags (with legacy single-dash compatibility) into `configs.Config`; NWAFU defaults are `vpn.nwafu.edu.cn`, `auth/psw`, and `LDAP`.
2. `underlay.Dialer` (`underlay/`, `internal/platformdialer/`) handles optional interface binding, local gateway DNS resolution, IP exclusion, and debug PCAP capture; `internal/keylog/` optionally writes NSS TLS key logs.
3. `client/atrust` authenticates via `client/atrust/auth` + `client/authchallenge`, handles periodic session refresh (`authConfig`), fetches resources/nodes (WAN/LAN groups, multi-IP DNS, `EnableTCPPrefL3`), and establishes the Sangfor L3 or TCP tunnel behind `client.Client` with live virtual-IP updates.
4. `stack.Stack` selects `stack/gvisor` by default, `stack/tun` for an OS TUN device, or `stack/tcptunnel` for TCP-only mode.
5. `resolve.Resolver` applies multi-ancestor domain trie matching, custom records, round-robin multi-IP DNS resources, coordinated deduplicated queries with hedged TCP fallback, and cache. Managed browser mode uses fixed-endpoint encrypted DoH when the gateway supplies no DNS, preventing local Fake-IP contamination.
6. `dial.Dialer` decides VPN versus direct routing from server-issued IP/domain resources and `EnableTCPPrefL3`. Managed browser mode is VPN-only: unauthorized destinations are rejected rather than falling back to the host network.
7. `service` starts SOCKS5, HTTP, DNS, Shadowsocks, TCP/UDP forwarding, and keep-alive goroutines. Managed browser mode instead creates one private loopback HTTP proxy and an application-grouped resource home page (`service/browser_home.go`).
8. `internal/managedbrowser` launches an isolated Chromium profile through that proxy; browser exit triggers normal terminal cleanup.
9. `internal/hook_func` performs startup checks and ordered cleanup on browser exit, session invalidation, or process signals.

LDAP flow: `/passport/v1/auth/psw` → `/controller/v1/public/reportEnv` → `/passport/v1/auth/authCheck` → `auth/token` subtype `totp` → `/passport/v1/auth/token` → resource/node setup. Preserve `authId`, `taskId`, `isPrevEffect`, subtype mapping, CSRF headers, anti-MITM checks, and routing context metadata.

WeCom flow: `authConfig` supplies the app, agent, redirect URI, and `state`; `auth/qywechat` fetches and polls the WeCom QR session, exposes optional loopback WebUI/CLI/file outputs, validates the synthesized callback, exchanges it for the portal ticket, then resumes normal authentication and resource/node setup. Keep callback host/path/domain/state validation and WebUI status feedback intact.

## Key Directories

- `client/atrust/`: aTrust session, periodic refresh, resources, WAN/LAN nodes, binary L3/TCP protocols.
- `client/atrust/auth/`: Password, SMS, WeCom QR callback, CAS/OAuth2, enhanced auth, device trust, anti-MITM, and TOTP/RADIUS secondary authentication.
- `client/authchallenge/`: Structured interactive challenge handler (CLI + hardened loopback captcha & verification code WebUIs).
- `underlay/`: Underlay TCP/UDP dialer, gateway DNS resolver, interface binding, and optional debug PCAP writer.
- `stack/`: gVisor, native TUN (Windows/macOS/Linux/Android), and TCP-only stacks behind `stack.Stack`.
- `dial/`: Resource-aware VPN/direct routing, `EnableTCPPrefL3` selection, and optional upstream proxies.
- `resolve/`: Reverse-trie domain resource matching, multi-IP DNS resources, coordinated resolution, fake-IP mapping, and cache.
- `service/`: Proxy, resource portal, DNS, forwarding, Shadowsocks, and keep-alive listeners.
- `configs/`: Runtime `koanf` schema, `Default()`, and pointer-based `ConfigTOML` schema.
- `embed/` & `cmd/libnwafuconnect/`: In-process Go embedding API and C ABI shared/static library (`NcStart`/`NcSubmit`/`NcStatus`/`NcStop`/`NcFree`).
- `mobile/`: Android `gomobile` aTrust + `VpnService` TUN bindings (`//go:build android`).
- `cmd/nwafu-connect-desktop/` & `cmd/nwafu-connect-proxy/`: Fyne desktop tray client and OpenSSH stdio proxy helper.
- `internal/`: Lifecycle hooks, IP pool, IP resource interval tree, TLS/TCP ping, keylog, platform socket binding, desktop controller/config/settings, autostart, managed browser launcher, DNS interfaces, raw packet helpers.
- `packaging/` & `scripts/`: Windows Inno Setup, macOS plist, Linux `.desktop`, and `build-lib.sh` library packaging scripts.
- `docs/`: Docker and service deployment guides in Chinese and English.
- `.github/workflows/`: CI (`build.yml`), unified full-platform tag release (`release.yml`), and optional Docker publishing (`docker.yml`).

## Development Commands

There is no Makefile or configured third-party linter. Use Go tools directly:

```bash
go mod download
go run . -config config.toml
CGO_ENABLED=0 go build -o nwafu-connect .
CGO_ENABLED=0 go build -o nwafu-connect-proxy ./cmd/nwafu-connect-proxy
go build ./cmd/nwafu-connect-desktop
go test -race -count=1 ./...
go vet ./...
gofmt -w path/to/changed.go
docker compose up --build -d
```

Live gateway discovery (off-campus only) is safe without credentials:

```bash
go run . -auth-info
```

Never run credentialed login with `-debug-dump`, `-debug-pcap-file`, or `-debug-tls-log-file` unless protocol payload logging is explicitly required and controlled.

## Code Conventions & Common Patterns

- Apply `gofmt`; package names are lowercase, constructors use `New...`, and exported names preserve Go initialisms (`IP`, `DNS`, `HTTP`, `TOTP`).
- Keep auth behavior in `client/atrust/auth`, tunnel behavior in `client/atrust`, and transport behavior behind `stack.Stack`; services must not duplicate protocol decisions.
- Never call `log.Fatalf` inside `client/atrust` library paths (especially on session invalidation `10000004`/`75500002`); return wrapped `auth.ErrSessionInvalid` and invoke `OnSessionInvalid` so `embed/` and CLI cleanup behave safely.
- Dependencies are assembled explicitly in `main.go`; there is no DI framework or central state store.
- Wrap causes with `fmt.Errorf("context: %w", err)` and compare sentinel errors with `errors.Is`.
- Propagate cancellation with `context.Context`, register long-lived cleanup through `internal/hook_func`, and protect shared connection/session maps with nearby `sync` patterns.
- Routing metadata uses context values for resolved hosts, domain resources, IP resources, and fake IPs. Do not bypass `resolve.Resolver` or `dial.Dialer`.
- `configs.Config` (`koanf` tags) and `configs.ConfigTOML` (pointer fields) must stay in sync with `configs.Default()`, `init.go`, and `config.toml.example`.
- Packet code mutates wire buffers directly. Preserve framing lengths, endianness, checksums, ownership, and bounds checks.
- Use the repository `log` package. Never log passwords, TOTP seeds/tokens, cookies, sign keys, or raw auth responses outside deliberate debug mode.
- UI surfaces (WebUIs in `service/browser_home.go`, `client/authchallenge/`, `client/atrust/auth/qywechat.go`, and desktop theme/settings) follow the design language in `DESIGN.md` (single `#0066cc` Action Blue accent, `#0071e3` focus ring, flat alternating surfaces, zero decorative gradients or card shadows).

## Runtime/Tooling Preferences

- Use Go `1.25.6`+ and Go modules from `go.mod`/`go.sum`.
- Production CLI builds are pure Go with `CGO_ENABLED=0`; preserve cross-platform compilation across all 17 targets.
- Do not reintroduce EasyConnect, Node/Bun tooling, vendoring, or a separate task runner.
- `config.toml` contains credentials; `client_data.json` contains aTrust cookies/device state. Both are ignored and should be mode `0600`.
- Docker publishing is disabled unless repository variable `DOCKER_IMAGE` is configured.

## Testing & QA

Tests use the standard `testing` package and live beside source. Never require live credentials or live `vpn.nwafu.edu.cn` connectivity in permanent tests (use `httptest` and loopback fixtures). Before submitting, run `gofmt` on changed Go files, `go test -race -count=1 ./...`, `go vet ./...`, and `CGO_ENABLED=0 go build .`.
