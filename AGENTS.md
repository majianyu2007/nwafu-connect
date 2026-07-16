# Repository Guidelines

## Project Overview

NWAFU Connect is a Go command-line aTrust client specialized for Northwest A&F University's `vpn.nwafu.edu.cn` gateway. It authenticates with LDAP/password/TOTP or the gateway's SMS flow, consumes server-issued resource rules, and exposes SOCKS5, HTTP, DNS, forwarding, TCP-tunnel, or TUN access. The Go module and canonical repository are `github.com/majianyu2007/nwafu-connect`.

## Architecture & Data Flow

1. `init.go` merges CLI flags or TOML into `configs.Config`; NWAFU defaults are `vpn.nwafu.edu.cn`, `auth/psw`, and `LDAP`.
2. `client/atrust` authenticates, completes secondary authentication, fetches resources/nodes, and establishes the Sangfor L3 or TCP tunnel behind `client.Client`.
3. `stack.Stack` selects `stack/gvisor` by default, `stack/tun` for an OS TUN device, or `stack/tcptunnel` for TCP-only mode.
4. `resolve.Resolver` applies custom records, server domain/DNS resources, and cache. Managed browser mode uses fixed-endpoint encrypted DoH instead of the host resolver when the gateway supplies no DNS, preventing local Fake-IP contamination.
5. `dial.Dialer` normally decides VPN versus direct routing from server-issued IP/domain resources. Managed browser mode is VPN-only: unauthorized destinations are rejected rather than falling back to the host network.
6. `service` starts SOCKS5, HTTP, DNS, Shadowsocks, forwarding, and keep-alive goroutines. Managed browser mode instead creates one private loopback HTTP proxy and an application-grouped resource home page.
7. `internal/managedbrowser` launches an isolated Chromium profile through that proxy; browser exit triggers normal terminal cleanup.
8. `internal/hook_func` performs startup checks and ordered cleanup on browser exit or process signals.

LDAP flow: `/passport/v1/auth/psw` → `/controller/v1/public/reportEnv` → `/passport/v1/auth/authCheck` → `auth/token` subtype `totp` → `/passport/v1/auth/token` → resource/node setup. Preserve `authId`, `taskId`, subtype mapping, CSRF headers, and routing context metadata.

WeCom flow: `authConfig` supplies the app, agent, redirect URI, and `state`; `auth/qywechat` fetches and polls the WeCom QR session, exposes optional loopback WebUI/CLI/file outputs, validates the synthesized callback, exchanges it for the portal ticket, then resumes normal authentication and resource/node setup. Keep callback host/path/domain/state validation and WebUI status feedback intact.

## Key Directories

- `client/atrust/`: aTrust session, resources, nodes, binary L3/TCP protocols.
- `client/atrust/auth/`: Password, SMS, WeCom QR callback, captcha, device trust, and TOTP secondary authentication.
- `stack/`: gVisor, native TUN, and TCP-only stacks behind `stack.Stack`.
- `dial/`: Resource-aware VPN/direct routing and optional upstream proxies.
- `resolve/`: DNS resource matching, fake-IP mapping, and cache.
- `service/`: Proxy, DNS, forwarding, Shadowsocks, and keep-alive listeners.
- `configs/`: Runtime and pointer-based TOML schemas.
- `internal/`: Lifecycle hooks, IP pool, ping, DNS interfaces, raw packet helpers.
- `internal/managedbrowser/`: Windows/macOS/Linux Chromium discovery, isolated profile arguments, and child-process lifecycle.
- `docs/`: Docker and service deployment guides in Chinese and English.
- `.github/workflows/`: Cross-platform builds and optional Docker publishing.

## Development Commands

There is no Makefile or configured third-party linter. Use Go tools directly:

```bash
go mod download
go run . -config config.toml
go build -o nwafu-connect .
go test ./...
go vet ./...
gofmt -w path/to/changed.go
docker compose up --build -d
```

Live gateway discovery is safe without credentials:

```bash
go run . -auth-info
```

Never run credentialed login with `-debug-dump` unless protocol payload logging is explicitly required and controlled.

## Code Conventions & Common Patterns

- Apply `gofmt`; package names are lowercase, constructors use `New...`, and exported names preserve Go initialisms (`IP`, `DNS`, `HTTP`, `TOTP`).
- Keep auth behavior in `client/atrust/auth`, tunnel behavior in `client/atrust`, and transport behavior behind `stack.Stack`; services must not duplicate protocol decisions.
- Dependencies are assembled explicitly in `main.go`; there is no DI framework or central state store.
- Wrap causes with `fmt.Errorf("context: %w", err)` and compare sentinel errors with `errors.Is`.
- Propagate cancellation with `context.Context`, register long-lived cleanup through `internal/hook_func`, and protect shared connection/session maps with nearby `sync` patterns.
- Routing metadata uses context values for resolved hosts, domain resources, and fake IPs. Do not bypass `resolve.Resolver` or `dial.Dialer`.
- `configs.ConfigTOML` uses pointer fields to distinguish an omitted key from a zero value. Update schema, defaults/flags, and `config.toml.example` together.
- Packet code mutates wire buffers directly. Preserve framing lengths, endianness, checksums, ownership, and bounds checks.
- Use the repository `log` package. Never log passwords, TOTP seeds/tokens, cookies, sign keys, or raw auth responses outside deliberate debug mode.

## Important Files

- `main.go`: aTrust composition, stack selection, services, shutdown.
- `init.go`: NWAFU defaults, flags, TOML parsing, auth/device utility commands.
- `configs/config.go`: Configuration schema.
- `config.toml.example`: Canonical NWAFU LDAP/TOTP example.
- `client/client.go`: VPN client/resource contract.
- `client/atrust/client.go`: Login-to-resource orchestration.
- `client/atrust/auth/auth.go`: Authentication chain state machine.
- `client/atrust/auth/totp.go`: NWAFU TOTP generation and token submission.
- `client/atrust/auth/request.go`: Auth-config/check response parsing.
- `stack/stack.go`, `dial/dialer.go`, `resolve/resolver.go`: Transport contracts and routing decisions.
- `Dockerfile`, `docker-compose.yml`, `.github/workflows/*.yml`: Build/deployment definitions.

## Runtime/Tooling Preferences

- Use Go `1.25.6` and Go modules from `go.mod`/`go.sum`.
- Production builds are pure Go with `CGO_ENABLED=0`; preserve cross-platform compilation.
- Do not reintroduce EasyConnect, Node/Bun tooling, vendoring, or a separate task runner.
- `config.toml` contains credentials; `client_data.json` contains aTrust cookies/device state. Both are ignored and should be mode `0600`.
- Docker publishing is disabled unless repository variable `DOCKER_IMAGE` is configured.

## Testing & QA

Tests use the standard `testing` package and live beside source. Current coverage includes graph captcha canonicalization, aTrust tunnel error classification, and TOTP service/payload mapping. There is no coverage threshold or network integration suite.

For auth changes, test response-to-step mapping and exact request fields with `httptest`; never require live credentials in permanent tests. Before submitting, run `gofmt` on changed Go files, `go test ./...`, `go vet ./...`, and `go build .`. For gateway-facing changes, additionally run `-auth-info`; a controlled live smoke test may use ignored local configuration but must not expose secrets in logs or chat.
