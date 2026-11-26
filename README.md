# NWAFU Connect

> **免责声明**
>
> 本程序**按原样提供**，与西北农林科技大学及深信服无隶属关系。作者不对程序的正确性或可靠性提供保证，请遵守学校网络和账户管理规定，使用本程序产生的一切后果由使用者自行承担。**请勿提交或分享账号、密码、TOTP 密钥、`config.toml` 或 `client_data.json`。**

---

中文 | [English](README_en.md)

NWAFU Connect 是面向西北农林科技大学 `vpn.nwafu.edu.cn` 的第三方 **aTrust** 开源客户端（桌面托盘 GUI + 命令行 CLI + 嵌入式 C/Go/移动端核心库）。本仓库基于上游 [zju-connect](https://github.com/Mythologyli/zju-connect) 与 [EasierConnect](https://github.com/lyc8503/EasierConnect) 演进而来，已移除学校不再使用的 EasyConnect 协议，专注于 NWAFU 的 aTrust 登录、受管浏览器、本地代理（SOCKS5 / HTTP / Shadowsocks / DNS / 端口转发）和系统 TUN 虚拟网卡接入。

> **校园网环境提示**
>
> **在校园网内部无法直接连接 `vpn.nwafu.edu.cn`**，且校园网内本就可以直接访问校内资源，无需开启 VPN。仅在校外网络（蜂窝热点、家庭宽带等）访问校内资源时使用本客户端；桌面版在检测到网关不可达时会自动暂停重连并给出提示。

## 下载与多平台支持

每次发布（`v*` Tag）均通过 GitHub Actions 自动构建并附带 `SHA256SUMS.txt` 校验文件：

| 平台 | 发布包形态 | 覆盖架构 | 说明 |
| --- | --- | --- | --- |
| **Windows 桌面版** | 安装版（`-setup.exe`）与绿色便携版（`-portable.zip`） | `amd64`, `arm64` | 系统托盘 + 设置窗口 + 一键受管浏览器 + `nwafu-connect` + `nwafu-connect-proxy` |
| **macOS 桌面版** | 安装镜像（`.dmg`）与便携版（`-portable.zip`） | `arm64` (Apple Silicon), `amd64` (Intel) | 原生 `.app` 应用包（见下方解除隔离命令） |
| **Linux 桌面版** | 安装包（`.deb`）与便携版（`-portable.tar.gz`） | `amd64`, `arm64` | 系统托盘 + 桌面入口 + 受管浏览器 |
| **全平台命令行 CLI** | `nwafu-connect-<os>-<arch>.zip`（17 个目标） | Windows (`amd64/386/arm64`)、macOS (`amd64/arm64`)、Linux (`amd64/386/arm64/armv5/armv6/armv7/riscv64/mips/mipsle/mips64/mips64le`) | 纯 Go 零 CGO，含 `nwafu-connect`、`nwafu-connect-proxy` 与示例配置 |
| **Android 核心库** | `-android-jnilibs.zip`（C ABI `.so`）与 `-android-aar.zip`（`gomobile` `VpnService` `.aar`） | `arm64-v8a`, `armeabi-v7a`, `x86_64` | 供宿主 Android App / Flutter / Kotlin 集成代理或 TUN VPN |
| **Apple 核心库** | `-ios-xcframework.zip` 与 `-macos-universal-lib.zip` | iOS 真机/模拟器 `arm64`、`macOS` Universal (`arm64`+`amd64`) | 静态 `NwafuConnect.xcframework` 与通用动态库，供宿主 iOS/macOS App 集成 |
| **Docker 镜像** | 多架构 Distroless 镜像 | `linux/amd64`, `linux/arm64` | 见 [`docs/docker.md`](docs/docker.md) |

> macOS 首次运行未公证应用/命令行二进制前，请在终端解除隔离属性：
> ```bash
> sudo xattr -rd com.apple.quarantine "/Applications/NWAFU Connect.app"
> # 或对 CLI 二进制：
> sudo xattr -rd com.apple.quarantine ./nwafu-connect ./nwafu-connect-proxy
> ```

### GUI 客户端选择建议

- **NWAFU Connect Desktop（推荐）**：直接在本仓库 [Releases](https://github.com/majianyu2007/nwafu-connect/releases) 下载对应系统的安装版或便携版。开箱预置西农网关 `vpn.nwafu.edu.cn:443`、支持 LDAP+TOTP / 短信 / 企业微信扫码、一键拉起隔离受管浏览器与校内资源门户、支持托盘复制本地代理地址与休眠唤醒自动重连。
- **第三方兼容 aTrust GUI 客户端**：如果你习惯使用通用多校 VPN GUI，也可以尝试 [Chenx Dust](https://github.com/chenx-dust) 开发的 [EZ4Connect](https://github.com/chenx-dust/EZ4Connect)（支持 aTrust 协议）或 [kowyo](https://github.com/kowyo) 开发的 [hitsz-connect-verge](https://github.com/kowyo/hitsz-connect-verge)。配置时请将协议选为 **aTrust**，服务器地址设为 `vpn.nwafu.edu.cn:443`，LDAP 登录域填写 `LDAP`。
- **移动端独立 App（APK / 未签名 IPA）**：当前仓库已直接提供可复用的 Android `.aar` / `jniLibs .so` 与 iOS `NwafuConnect.xcframework`（以及 `embed` Go 包与 `scripts/build-lib.sh` 构建脚本），供移动端外壳直接接入代理或 `VpnService` / `NEPacketTunnelProvider`。独立 Android APK 与未签名 iOS IPA 外壳应用计划在后续独立 App 工程中持续跟进。

## 认证方式支持状态

在校外网络环境下，可通过 `nwafu-connect -auth-info` 查询网关当前公布的认证方式：

| 认证方式 | aTrust 标识 (`auth_type` / `login_domain`) | 状态 |
| --- | --- | --- |
| LDAP（学号、密码、TOTP） | `auth/psw` / `LDAP` | 已完成真实账号验证 |
| 企业微信扫码（WebUI / CLI / PNG） | `auth/qywechat` / `wechat` | 已完成真实账号全流程验证 |
| 手机号 + 短信验证码 | `auth/smsCheckCode` / `sms73926` | 已实现交互流程（支持本地浏览器/终端输入验证码），待真实号码验证 |
| CAS / HTTPS OAuth2（通用 aTrust） | `auth/cas`、`auth/httpsOauth2` | 已同步上游交互回调与增强认证支持 |

## 使用模式对比

| 使用场景 | 推荐模式 | 特点 |
| --- | --- | --- |
| 日常查论文、进教务/图书馆等校内网页 | **受管浏览器模式**（`browser_mode = true` 或桌面版默认模式） | 隔离独立 Chromium Profile，不改系统代理；严格 VPN-only 防泄漏；缺失网关 DNS 时自动使用加密 DoH 避免 Fake-IP 污染；自带资源导航门户 |
| 终端、浏览器扩展、分流工具（Clash/Mihomo 等）接入 | **SOCKS5 / HTTP 代理模式**（CLI 默认） | 默认监听 `127.0.0.1:1080`（SOCKS5）与 `127.0.0.1:1081`（HTTP），支持服务端 IP/域名规则自动分流 |
| 需要全系统透明访问或不支持代理的桌面客户端 | **TUN 虚拟网卡模式**（`-tun-mode`） | 创建系统虚拟网卡并自动下发路由与 Fake-IP DNS 劫持（需管理员权限） |
| 仅需要 SSH / SFTP / Git 等 TCP 工具接入受管会话 | **`nwafu-connect-proxy` 助手** | 配合 OpenSSH `ProxyCommand` 复用受管浏览器会话的私有回环代理 |

## 快速开始（命令行 CLI）

从 [Releases](https://github.com/majianyu2007/nwafu-connect/releases) 下载对应平台压缩包，或本地使用 Go 1.25.6+ 编译：

```bash
git clone https://github.com/majianyu2007/nwafu-connect.git
cd nwafu-connect
cp config.toml.example config.toml
chmod 600 config.toml
```

### 1. LDAP + TOTP（推荐自动化）

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

也可以通过环境变量传入敏感凭据，避免明文落盘：

```bash
NWAFU_CONNECT_PASSWORD="你的密码" NWAFU_CONNECT_TOTP_SECRET="你的密钥" \
  ./nwafu-connect --config config.toml
```

### 2. 企业微信扫码登录（WebUI / CLI / PNG）

无需配置账号密码，同时支持本地回环 Web 状态页、终端半块字符二维码和 `0600` 权限 PNG 文件三种展示方式：

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

### 3. 手机号 + 短信验证码

```toml
server_address = "vpn.nwafu.edu.cn"
server_port = 443
auth_type = "auth/smsCheckCode"
login_domain = "sms73926"
phone = "86-13800138000"
client_data_file = "client_data.json"
```

若网关触发点选图形验证码，客户端会自动打开仅监听 `127.0.0.1` 的本地交互页面；在无交互终端环境下，短信验证码也会自动回退到本地回环网页输入。

### 4. 受管浏览器模式

```toml
browser_mode = true
browser_url = "" # 留空打开按应用分组的校内资源首页
browser_path = "" # 留空自动发现 Chrome、Edge、Chromium 或 Brave
```

启动后不会修改系统代理或污染日常浏览器数据；所有 HTTP/HTTPS 请求走私有随机回环代理，仅允许访问 aTrust 网关授权资源，关闭所有受管浏览器窗口后自动清理临时 Profile 并断开隧道。

### 5. TUN 虚拟网卡模式注意事项

1. 需要以管理员 / `root` 权限运行。
2. Windows 用户请前往 [Wintun 官网](https://www.wintun.net) 下载对应架构的 `wintun.dll`，放置于 `nwafu-connect.exe` 同目录。
3. aTrust 协议下推荐使用：
   ```bash
   sudo ./nwafu-connect --config config.toml --tun-mode --add-route --dns-hijack --fake-ip
   ```
   不使用 `--dns-hijack --fake-ip` 时，经过 TUN 网卡的域名 TCP 流量可能无法匹配 aTrust 域名资源规则。
4. 若本机同时运行其他开启了 TUN + Fake-IP 的代理工具，请务必正确配置分流规则，并谨慎开启 `auto_detect_interface`。

## 配置优先级与关键参数

配置按 **内置默认值 < TOML 配置文件 < 环境变量 < 显式命令行参数** 的顺序合并（仅显式传入的命令行参数会覆盖前者，例如 `--tun-mode=false` 可关闭配置文件中的设置；单横杠旧参数如 `-config`、`-auth-info` 仍完全兼容）。

所有配置键均支持 `NWAFU_CONNECT_` 前缀的大写环境变量，例如：
- `NWAFU_CONNECT_CONFIG=/path/to/config.toml`
- `NWAFU_CONNECT_PASSWORD=...`、`NWAFU_CONNECT_TOTP_SECRET=...`
- `NWAFU_CONNECT_SESSION_REFRESH_INTERVAL=1800`
- `NWAFU_CONNECT_TCP_PORT_FORWARDING=127.0.0.1:9898-10.10.98.98:80`
- `NWAFU_CONNECT_CUSTOM_DNS=library.nwafu.edu.cn:10.0.0.10`

常用新增与核心选项（完整列表运行 `./nwafu-connect -h`）：

- `--session-refresh-interval`：aTrust 会话定时刷新间隔（秒），默认 `1800`，`0` 禁用；会话刷新会自动回写 `client_data_file` 中的最新 Cookie。
- `--update-best-nodes-interval`：最优接入节点探测间隔（秒），默认 `300`，`0` 禁用。
- `--secondary-dns-server`：备用 DNS，默认 `auto`（优先使用网关策略下发的第二 DNS，否则回退 `114.114.114.114`；留空则使用系统 DNS）。
- `--local-dns-server`：用于解析 VPN 网关域名本身的底层 DNS（格式 `IP` 或 `IP:port`），适合复杂网络或防环路场景。
- `--bind-interface` / `--auto-detect-interface`：将 VPN 底层连接绑定到指定物理网卡或自动探测网卡（与另一款 Fake-IP VPN 共存时请勿开启自动探测）。
- `--proxy-all`：强制所有流量走 VPN 隧道。
- `--trust-device` / `--untrust-device`：基于已保存的 `--client-data-file` 将当前设备加入或移出 aTrust 授信终端列表。
- `--debug-pcap-file` / `--debug-tls-log-file`：分别输出底层 TCP 重建 PCAP 与 NSS TLS Key Log，用于受控 Wireshark 抓包排障（含敏感会话密钥，切勿泄露）。

## SSH / SFTP 等 TCP 客户端接入

受管浏览器与 CLI 均附带 `nwafu-connect-proxy` stdio 代理助手，可将 OpenSSH / SFTP / Git 等接入当前代理会话：

```bash
ssh -o 'ProxyCommand="/Applications/NWAFU Connect.app/Contents/MacOS/nwafu-connect-proxy" --proxy 127.0.0.1:63665 --target %h:%p' user@10.133.16.10
```

## 嵌入式库与移动端构建

- Go 宿主进程可直接导入 [`embed`](embed/embed.go) 包，通过 `embed.Prompter` 接管图形验证码与短信/TOTP 交互，在进程内拉起 gVisor 用户态隧道与本地 HTTP 代理。
- C ABI / Flutter / Swift / Kotlin 宿主可使用 `cmd/libnwafuconnect` 与 [`scripts/build-lib.sh`](scripts/build-lib.sh)（支持 `host`、`macos`、`android`、`ios`）。
- Android `VpnService` 宿主也可使用 `mobile` 包（`gomobile bind -target=android ./mobile`）直接将 TUN `fd` 接入 aTrust L3 隧道。

## 安全与部署文档

- `config.toml` 和 `client_data.json` 均已加入 `.gitignore`，请保持 `0600` 权限，切勿公开分享。
- 生产环境建议将代理绑定在 `127.0.0.1`，并通过配置文件或环境变量传递凭据，避免出现在进程命令行参数中。
- Docker 部署见 [`docs/docker.md`](docs/docker.md)，系统后台服务部署见 [`docs/service.md`](docs/service.md)。

## 致谢

- [zju-connect](https://github.com/Mythologyli/zju-connect)
- [EasierConnect](https://github.com/lyc8503/EasierConnect)
- [EZ4Connect](https://github.com/chenx-dust/EZ4Connect)
