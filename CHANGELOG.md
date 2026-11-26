# Changelog

本文件记录 NWAFU Connect 的重要功能、行为变更与缺陷修复。

格式参考 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循[语义化版本](https://semver.org/lang/zh-CN/)。

## [Unreleased]

## [1.5.0] - 2026-11-26

### Added

- 同步上游 `zju-connect` 全部 aTrust 核心增强（继续保持纯 aTrust 架构，不引入 EasyConnect）：
  - 新增 `underlay/`、`internal/platformdialer/` 与 `internal/keylog/`：支持底层网卡绑定（`bind_interface` / `auto_detect_interface`）、网关独立 DNS（`local_dns_server`）、调试 PCAP 导出（`debug_pcap_file`）与 NSS TLS Key Log（`debug_tls_log_file`）。
  - 新增 aTrust 定时会话刷新（`session_refresh_interval`）与自动回写最新 Cookie、防中间人校验（anti-MITM）、服务端清单解析（TCP 隧道 0-RTT）、增强认证链、设备绑定、WAN/LAN 节点组与动态虚拟 IP 实时更新。
  - 新增 `koanf` + `pflag` 统一配置合并机制，支持 `NWAFU_CONNECT_*` 环境变量与 `默认 < TOML < 环境变量 < 显式命令行参数` 优先级，同时兼容旧单横杠参数。
  - 新增 `client/authchallenge/` 统一交互挑战接口与本地回环浏览器输入页。
  - 新增 `mobile/mobile_android.go`（纯 aTrust Android `VpnService` `gomobile` 绑定）。
- 扩展多平台全架构发布流水线（`.github/workflows/release.yml`）：统一在 `v*` Tag 发布全部 17 架构 CLI、Windows/macOS/Linux `amd64`/`arm64` 安装版与便携版、Android `jniLibs`/`aar`、iOS `NwafuConnect.xcframework`、macOS 通用动态库及 `SHA256SUMS.txt`。
- 桌面托盘新增“断开连接”、一键复制本地代理地址以及校园网网关不可达友好提示。
- 新增安全的本地验证码输入页，在无可交互终端时接收短信验证码，并提供随机表单令牌、输入校验、超时和一次提交保护。
- 完善企业微信二维码登录：使用网关下发配置，支持二维码下载、终端渲染、本地状态页、轮询和严格的回调校验。
- 新增 IPv4 资源区间索引和域名后缀索引，供拨号器、L3、TCP tunnel、TUN 与 DNS 解析共用。
- 为 SSH/SFTP 等 TCP 客户端提供 `nwafu-connect-proxy` stdio 代理助手及跨平台命令复制指引。
- 补充认证、资源解析、DNS、代理、转发、隧道、桌面生命周期和私有文件写入的回归测试。

### Changed

- 依据 `DESIGN.md` 全面重构并美化所有 WebUI（受管浏览器资源门户、点选图形验证码页、本地验证码输入页、企业微信扫码状态页）及 Fyne 桌面主题与设置窗口，采用单主色 `#0066cc`、平铺交替表面、去阴影与胶囊交互风格。
- DNS 解析器同步上游多祖先域名 Trie、多 IP 轮询、去重并发查询（支持取消）及 300ms TCP 对冲回退，并保留受管浏览器模式固定端点 DoH 隔离能力。
- 拨号器与网络栈同步 `EnableTCPPrefL3` 协议优选，TCP 端口转发改经 `dial.Dialer` 路由。
- 重做受管浏览器资源首页，采用清晰的会话状态、资源搜索、类型筛选、结果计数、空状态和显式地址操作。
- 资源首页首屏仅渲染 18 项，其余资源按需加载；搜索和筛选通过动画帧合并更新，避免大列表布局卡顿。
- 受管浏览器改为严格 VPN-only 路由：网关未授权的目的地会被拒绝，不再回退到宿主机网络。
- 仅允许资源首页的精确回环 origin 绕过浏览器代理，其余流量仍通过受管代理。
- 认证流程改为有界状态链，统一推进密码、短信、自定义短信、TOTP 和后续认证步骤。
- TOML 配置使用指针字段区分“未配置”和显式零值；显式命令行参数在加载配置文件后仍具有最高优先级。
- 节点探测改为并发、有界且可停止的 TCP 探测，并在全部探测失败时回退到第一个合法节点。
- 服务关闭改为逆注册顺序，并统一传播启动、运行和清理错误。

### Fixed

- 修复 `main.go` 中 TUN 模式未赋值 `vpnStack = vpnTUNStack` 导致的必现空指针崩溃，以及 `clientIP == nil` 边界问题。
- 修复 `client/atrust` 在会话失效（`10000004`/`75500002`）时直接 `log.Fatalf` 终止进程的问题，改为返回 `auth.ErrSessionInvalid` 并通过回调优雅关闭（兼容 CLI 与 `embed/` 宿主）。
- 修复 `stack/tun/stack_darwin.go` 错误使用 `IP_RECVIF` 而非 `IP_BOUND_IF` 的 socket 绑定问题，以及 `gvisor.Stack` 异常退出时的资源泄漏。
- 修复 `service/shadowsocks.go` UDP NAT 映射键冲突问题。
- 修复 `embed/` 在网关不可达（例如校园网内部）时重复清零缓存重试、关闭未等待协议栈退出及 Windows 原子文件替换问题。
- 修复 `cmd/libnwafuconnect` C ABI 空指针防护、提交校验返回值 `ok` 标记及持锁关闭旧会话问题。
- 修复 `cmd/nwafu-connect-proxy` 响应行缺少换行符时可能引发的无界内存读取问题。
- 修复 Windows 桌面端取消勾选开机自启时 `reg.exe delete` 报错、设置页保存覆盖高级 TOML 编辑、切换认证方式不联动登录域、托盘 45 秒超时提前放弃交互式登录、休眠唤醒离线误重连、UI 线程外通知调用、Windows 强制强杀子进程及未回收打开目录/链接子进程等桌面端问题。
- 修复原 `.github/workflows/desktop.yml` 使用 `GITHUB_TOKEN` 创建 Release 导致 `build.yml` 未触发、17 架构 CLI 资产缺失的问题，以及 `Dockerfile` 硬编码 `GOARCH=amd64` 影响 ARM64 镜像的问题。
- 修复资源首页脚本状态变量未定义，导致搜索、筛选和“显示更多”失效的问题。
- 修复图形验证码挑战被误判为认证失败，以及密码或短信响应缺少 ticket 仍被接受的问题。
- 修复 TOTP 下一服务、action、task ID、auth ID 和 token 字段映射不一致的问题。
- 修复认证 HTTP 状态、响应体读取错误和超大响应未被正确拒绝的问题。
- 修复登录结果未持久化最终 Cookie Jar 中 SID 与 Cookie 的问题。
- 修复空 Host、非法通配符、倒置端口范围、异常 IPv6 地址和畸形节点地址可能污染资源或触发崩溃的问题。
- 修复同一域名的多条端口规则互相覆盖，以及服务器下发多个 IP 时未全部加入 VPN 路由的问题。
- 修复域名后缀跨标签误匹配、缓存命中后丢失资源上下文、Fake-IP 优先级错误和 TTL 为零仍缓存的问题。
- 修复 Fake-IP 池耗尽时 panic、固定地址冲突和正反向映射残留的问题。
- 修复 TLS/隧道短写、握手无法取消、L3 关闭不能解除阻塞读取和 conntrack 认证超时残留的问题。
- 修复 TCP tunnel 手工拼接 JSON、未等待目标连接状态、空帧/未知帧处理错误和零长度 I/O 阻塞的问题。
- 修复 HTTP CONNECT 缓冲数据丢失、错误接受非成功状态、小响应未刷新及关闭时挂起拨号未取消的问题。
- 修复 HTTP 代理连接池和活跃隧道无界增长的问题。
- 修复 UDP 转发连接与排队内存无界增长、拨号失败后队列残留和非 LRU 淘汰的问题。
- 修复 DNS、HTTP、SOCKS、Shadowsocks 与转发服务监听失败却仍报告启动成功的问题。
- 修复桌面设置保存时覆盖高级 TOML 字段或已有凭据，以及休眠唤醒、网络恢复和陈旧浏览器状态处理不一致的问题。

### Security

- 认证客户端恢复标准 TLS 证书验证，并设置请求总超时。
- 本地资源页和验证码页限制回环 Host 与允许的方法，并设置 CSP、frame deny、nosniff、referrer policy 和权限策略。
- 浏览器代理地址必须是合法回环地址，起始页必须使用 HTTP 或 HTTPS。
- 客户端数据、浏览器状态和验证码图片使用私有目录、原子替换及 `0600` 权限。
- 受管浏览器临时资料目录会在退出后清理；持久资料目录不再使用弱化凭据存储参数。

### Performance

- 用区间树替代拨号和逐包路径中的完整 IP 资源线性扫描，用反向 trie 替代 DNS 域名资源线性扫描。
- 禁用调试时跳过数据包格式化、帧复制和十六进制输出；调试状态改为原子读取。
- UDP 转发增加全局字节预算、每连接队列上限、连接上限与 LRU 淘汰。
- HTTP 代理限制总空闲连接、单主机连接、响应头等待时间和活跃 CONNECT 隧道数量。

[Unreleased]: https://github.com/majianyu2007/nwafu-connect/compare/v1.5.0...HEAD
[1.5.0]: https://github.com/majianyu2007/nwafu-connect/compare/v1.4.1...v1.5.0
