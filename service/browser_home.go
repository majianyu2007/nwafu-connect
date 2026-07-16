package service

import (
	"context"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/internal/hook_func"
	"github.com/majianyu2007/nwafu-connect/log"
)

type browserHomeAddress struct {
	Host     string
	URL      string
	Protocol string
	Ports    string
}

type browserHomeResource struct {
	Name        string
	Description string
	SearchText  string
	Primary     browserHomeAddress
	Additional  []browserHomeAddress
}

type browserHomeData struct {
	Resources  []browserHomeResource
	SSHCommand string
}

var browserHomeTemplate = template.Must(template.New("browser-home").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>NWAFU Connect · 校内资源门户</title>
<style>
:root {
  color-scheme: light;
  --ink: #15231e;
  --muted: #64736c;
  --line: #dfe8e3;
  --paper: #f4f7f5;
  --green: #123f32;
  --green-2: #1d5b49;
  --gold: #e2b654;
}
* { box-sizing: border-box; }
body { margin: 0; background: var(--paper); color: var(--ink); font: 15px/1.55 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
button, input { font: inherit; }
.hero { position: relative; overflow: hidden; min-height: 360px; color: white; background: linear-gradient(135deg, #0c3026 0%, #164e3e 62%, #1d6650 100%); }
.hero::before, .hero::after { position: absolute; border-radius: 999px; content: ""; filter: blur(1px); opacity: .18; }
.hero::before { width: 520px; height: 520px; top: -310px; right: -90px; background: #f1ca72; }
.hero::after { width: 360px; height: 360px; bottom: -270px; left: 12%; border: 1px solid white; }
.wrap { position: relative; width: min(1160px, calc(100% - 40px)); margin: 0 auto; }
nav { display: flex; align-items: center; justify-content: space-between; padding: 28px 0; }
.brand { display: flex; align-items: center; gap: 12px; font-weight: 700; letter-spacing: -.02em; }
.mark { display: grid; width: 38px; height: 38px; place-items: center; border: 1px solid rgba(255,255,255,.28); border-radius: 11px; background: rgba(255,255,255,.1); font-size: 18px; }
.connected { display: flex; align-items: center; gap: 8px; padding: 7px 12px; border: 1px solid rgba(255,255,255,.18); border-radius: 999px; background: rgba(255,255,255,.08); color: #e5f3ed; font-size: 13px; }
.connected::before { width: 8px; height: 8px; border-radius: 50%; background: #77d69f; box-shadow: 0 0 0 4px rgba(119,214,159,.14); content: ""; }
.intro { display: grid; grid-template-columns: 1fr auto; gap: 48px; align-items: end; padding: 48px 0 82px; }
.eyebrow { margin-bottom: 14px; color: #e8c778; font-size: 12px; font-weight: 700; letter-spacing: .16em; text-transform: uppercase; }
h1 { max-width: 720px; margin: 0; font-size: clamp(36px, 5vw, 60px); line-height: 1.08; letter-spacing: -.045em; }
.lead { max-width: 700px; margin: 20px 0 0; color: #c7ddd4; font-size: 16px; }
.total { min-width: 156px; padding: 20px 24px; border: 1px solid rgba(255,255,255,.16); border-radius: 18px; background: rgba(255,255,255,.08); backdrop-filter: blur(10px); }
.total strong { display: block; font-size: 34px; line-height: 1; }
.total span { display: block; margin-top: 8px; color: #c7ddd4; font-size: 13px; }
.content { padding: 0 0 64px; }
.search-panel { display: flex; gap: 14px; align-items: center; margin-top: -32px; padding: 16px 18px; border: 1px solid rgba(24,67,53,.09); border-radius: 18px; background: white; box-shadow: 0 18px 50px rgba(21,57,45,.12); }
.search-icon { flex: 0 0 auto; width: 20px; color: #557268; }
#resourceSearch { width: 100%; border: 0; outline: 0; color: var(--ink); background: transparent; font-size: 16px; }
#resourceSearch::placeholder { color: #92a099; }
.shortcut { padding: 3px 8px; border: 1px solid var(--line); border-radius: 6px; color: #809087; background: #f8faf9; font-size: 12px; }
.section-head { display: flex; align-items: end; justify-content: space-between; gap: 20px; margin: 46px 0 18px; }
h2 { margin: 0; font-size: 22px; letter-spacing: -.025em; }
.section-head p { margin: 5px 0 0; color: var(--muted); }
#resultCount { color: var(--muted); font-size: 13px; white-space: nowrap; }
.grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; }
.card { min-width: 0; overflow: hidden; border: 1px solid var(--line); border-radius: 15px; background: white; transition: transform .18s ease, border-color .18s ease, box-shadow .18s ease; }
.card:hover { transform: translateY(-2px); border-color: #b8cec4; box-shadow: 0 12px 28px rgba(27,67,53,.08); }
.card-main { display: flex; gap: 13px; align-items: flex-start; min-height: 96px; padding: 17px 16px 14px; }
.resource-icon { display: grid; flex: 0 0 auto; width: 42px; height: 42px; place-items: center; border-radius: 12px; color: var(--green-2); background: #eaf3ef; font-size: 14px; font-weight: 800; }
.card-copy { min-width: 0; }
.resource-name { display: block; overflow: hidden; font-weight: 700; letter-spacing: -.01em; text-overflow: ellipsis; white-space: nowrap; }
.description { display: -webkit-box; overflow: hidden; margin-top: 4px; color: var(--muted); font-size: 12px; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
.address, .address-static { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 10px; align-items: center; padding: 10px 16px; border-top: 1px solid #edf2ef; color: var(--green-2); background: #fbfcfb; font-size: 12px; text-decoration: none; }
.address:hover { background: #f3f8f5; }
.host { overflow: hidden; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.meta { color: #839189; white-space: nowrap; }
details { border-top: 1px solid #edf2ef; background: #fbfcfb; }
summary { padding: 9px 16px; color: #587167; cursor: pointer; font-size: 12px; list-style-position: inside; }
details .address, details .address-static { padding-left: 28px; border-top-color: #f0f3f1; }
.empty { display: none; padding: 48px 24px; border: 1px dashed #c7d7d0; border-radius: 16px; color: var(--muted); text-align: center; }
.actions { display: flex; justify-content: center; margin-top: 24px; }
#showMore { padding: 10px 18px; border: 1px solid #bed0c7; border-radius: 10px; color: var(--green); background: white; cursor: pointer; font-weight: 650; }
#showMore:hover { border-color: var(--green-2); background: #f8fbf9; }
.protocol-guide { display: grid; grid-template-columns: 1fr auto; gap: 16px; align-items: center; margin-top: 20px; padding: 18px 20px; border: 1px solid #cfe0d8; border-radius: 15px; background: #edf6f2; }
.protocol-guide strong { display: block; margin-bottom: 3px; color: var(--green); }
.protocol-guide p { margin: 0; color: #5e756b; font-size: 12px; }
.protocol-guide code { display: block; overflow-x: auto; margin-top: 9px; padding: 8px 10px; border-radius: 7px; color: #dcece5; background: #173d31; white-space: nowrap; }
.copy-button { padding: 9px 14px; border: 1px solid #afc9bd; border-radius: 9px; color: var(--green); background: white; cursor: pointer; font-weight: 650; white-space: nowrap; }
.copy-button:hover { border-color: var(--green-2); }
.notice { display: grid; grid-template-columns: auto 1fr; gap: 14px; margin-top: 42px; padding: 20px; border: 1px solid #eadcb9; border-radius: 15px; background: #fffaf0; }
.notice-icon { display: grid; width: 36px; height: 36px; place-items: center; border-radius: 10px; color: #765817; background: #f5e6bd; font-weight: 800; }
.notice strong { display: block; margin-bottom: 3px; }
.notice p { margin: 0; color: #796b49; font-size: 13px; }
footer { margin-top: 42px; color: #7b8a83; font-size: 12px; text-align: center; }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0,0,0,0); white-space: nowrap; }
@media (max-width: 850px) {
  .intro { grid-template-columns: 1fr; gap: 26px; padding-top: 34px; }
  .total { width: fit-content; }
  .grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
@media (max-width: 560px) {
  .wrap { width: min(100% - 24px, 1160px); }
  nav { padding-top: 18px; }
  .connected { font-size: 0; }
  .connected::after { content: "已连接"; font-size: 12px; }
  .intro { padding-bottom: 68px; }
  .grid { grid-template-columns: 1fr; }
  .shortcut { display: none; }
  .section-head { align-items: start; flex-direction: column; gap: 8px; }
}
</style>
</head>
<body>
<header class="hero">
  <div class="wrap">
    <nav>
      <div class="brand"><span class="mark">N</span><span>NWAFU Connect</span></div>
      <div class="connected">aTrust 安全连接已建立</div>
    </nav>
    <div class="intro">
      <div>
        <div class="eyebrow">Campus Resource Gateway</div>
        <h1>校内资源，一站直达</h1>
        <p class="lead">浏览本次登录由学校 aTrust 网关授权的资源。点击资源后，NWAFU Connect 会自动通过对应的安全隧道访问。</p>
      </div>
      <div class="total"><strong>{{len .Resources}}</strong><span>项应用资源</span></div>
    </div>
  </div>
</header>
<main class="content wrap">
  <label class="search-panel" for="resourceSearch">
    <span class="search-icon">⌕</span>
    <span class="sr-only">搜索资源名称、说明或地址</span>
    <input id="resourceSearch" type="search" placeholder="搜索资源名称、说明或地址…" autocomplete="off">
    <span class="shortcut">⌘ K</span>
  </label>
  <section class="protocol-guide">
    <div><strong>SSH / SFTP 等 TCP 客户端</strong><p>使用随应用打包的 stdio 代理助手，将任意 TCP 客户端接入当前 aTrust 会话：</p><code id="sshCommand">{{.SSHCommand}}</code></div>
    <button class="copy-button" type="button" data-copy="sshCommand">复制命令</button>
  </section>
  <div class="section-head">
    <div><h2>全部资源</h2><p>资源权限由学校 aTrust 网关实时下发</p></div>
    <span id="resultCount">{{len .Resources}} 个结果</span>
  </div>
  <section id="resourceGrid" class="grid" aria-live="polite">
    {{range .Resources}}<article class="card" data-search="{{.SearchText}}"><div class="card-main"><span class="resource-icon">R</span><span class="card-copy"><span class="resource-name">{{.Name}}</span>{{if .Description}}<span class="description">{{.Description}}</span>{{end}}</span></div>{{if .Primary.URL}}<a class="address" href="{{.Primary.URL}}" target="_blank" rel="noopener noreferrer"><span class="host">{{.Primary.Host}}</span><span class="meta">{{.Primary.Protocol}} · {{.Primary.Ports}}</span></a>{{else}}<div class="address-static"><span class="host">{{.Primary.Host}}</span><span class="meta">{{.Primary.Protocol}} · {{.Primary.Ports}}</span></div>{{end}}{{if .Additional}}<details><summary>另外 {{len .Additional}} 个下发地址</summary>{{range .Additional}}{{if .URL}}<a class="address" href="{{.URL}}" target="_blank" rel="noopener noreferrer"><span class="host">{{.Host}}</span><span class="meta">{{.Protocol}} · {{.Ports}}</span></a>{{else}}<div class="address-static"><span class="host">{{.Host}}</span><span class="meta">{{.Protocol}} · {{.Ports}}</span></div>{{end}}{{end}}</details>{{end}}</article>{{end}}
  </section>
  <div id="emptyState" class="empty">没有找到匹配的授权资源，请尝试其他关键词。</div>
  <div class="actions"><button id="showMore" type="button">显示全部资源</button></div>
  <aside class="notice"><span class="notice-icon">i</span><div><strong>找不到需要的校内网站？</strong><p>NWAFU Connect 只能使用学校 aTrust 网关下发并授权的资源。若校内网站未出现在列表中，需要由学校网络管理员将它加入 aTrust 资源策略，客户端无法绕过网关权限。</p></div></aside>
  <footer>NWAFU Connect · 临时受管浏览器会话</footer>
</main>
<script>
(() => {
  const input = document.getElementById("resourceSearch");
  const cards = Array.from(document.querySelectorAll(".card"));
  const count = document.getElementById("resultCount");
  const empty = document.getElementById("emptyState");
  const more = document.getElementById("showMore");
  const initialLimit = 18;
  let expanded = false;
  function render() {
    const query = input.value.trim().toLowerCase();
    const matches = cards.filter(card => card.dataset.search.toLowerCase().includes(query));
    cards.forEach(card => { card.style.display = "none"; });
    const visible = query || expanded ? matches : matches.slice(0, initialLimit);
    visible.forEach(card => { card.style.display = ""; });
    count.textContent = matches.length + " 个结果";
    empty.style.display = matches.length ? "none" : "block";
    more.style.display = !query && matches.length > initialLimit ? "inline-block" : "none";
    more.textContent = expanded ? "收起资源" : "显示全部资源";
  }
  input.addEventListener("input", render);
  more.addEventListener("click", () => { expanded = !expanded; render(); });
  document.addEventListener("keydown", event => {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
      event.preventDefault();
      input.focus();
    }
  });
  document.querySelectorAll("[data-copy]").forEach(button => {
    button.addEventListener("click", async () => {
      const text = document.getElementById(button.dataset.copy).textContent;
      try {
        await navigator.clipboard.writeText(text);
      } catch (_) {
        const area = document.createElement("textarea");
        area.value = text;
        document.body.appendChild(area);
        area.select();
        document.execCommand("copy");
        area.remove();
      }
      button.textContent = "已复制";
      setTimeout(() => { button.textContent = "复制命令"; }, 1500);
    });
  });
  render();
})();
</script>
</body>
</html>`))

func StartBrowserHome(sourceResources []client.Resource, proxyAddress string) (string, error) {
	resources := make([]browserHomeResource, 0, len(sourceResources))
	for _, source := range sourceResources {
		addresses := make([]browserHomeAddress, 0, len(source.Addresses))
		searchParts := []string{source.Name, source.Description}
		for _, address := range source.Addresses {
			host := strings.TrimSpace(address.Host)
			if host == "" {
				continue
			}
			addresses = append(addresses, browserHomeAddress{
				Host:     host,
				URL:      browserAddressURL(host, address),
				Protocol: address.Protocol,
				Ports:    portRange(address.PortMin, address.PortMax),
			})
			searchParts = append(searchParts, host)
		}
		if len(addresses) == 0 {
			continue
		}
		name := strings.TrimSpace(source.Name)
		if name == "" {
			name = addresses[0].Host
		}
		resources = append(resources, browserHomeResource{
			Name:        name,
			Description: strings.TrimSpace(source.Description),
			SearchText:  strings.Join(searchParts, " "),
			Primary:     addresses[0],
			Additional:  addresses[1:],
		})
	}
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Name < resources[j].Name
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("start managed browser home page: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if err := browserHomeTemplate.Execute(w, browserHomeData{Resources: resources, SSHCommand: sshProxyCommand(proxyAddress)}); err != nil {
			log.Printf("Render managed browser home page failed: %v", err)
		}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	hook_func.RegisterTerminalFunc("CloseBrowserHome", func(ctx context.Context) error {
		shutdownContext, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("close managed browser home page: %w", err)
		}
		return nil
	})
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("Managed browser home page failed: %v", err)
		}
	}()
	return "http://" + listener.Addr().String() + "/", nil
}

func browserAddressURL(host string, resource client.ResourceAddress) string {
	if strings.HasPrefix(host, "*.") || strings.Contains(host, "/") || isIPRange(host) {
		return ""
	}
	if resource.Protocol != "tcp" && resource.Protocol != "all" {
		return ""
	}
	switch {
	case resource.PortMin <= 443 && resource.PortMax >= 443:
		return "https://" + host + "/"
	case resource.PortMin <= 80 && resource.PortMax >= 80:
		return "http://" + host + "/"
	default:
		return ""
	}
}

func isIPRange(host string) bool {
	parts := strings.Split(host, "-")
	return len(parts) == 2 && net.ParseIP(parts[0]) != nil && net.ParseIP(parts[1]) != nil
}

func sshProxyCommand(proxyAddress string) string {
	helperName := "nwafu-connect-proxy"
	if runtime.GOOS == "windows" {
		helperName += ".exe"
	}
	helper := helperName
	if executable, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(executable), helperName)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			helper = candidate
		}
	}
	if runtime.GOOS == "windows" {
		if strings.ContainsAny(helper, " \t") {
			helper = `\"` + helper + `\"`
		}
		return fmt.Sprintf(`ssh -o "ProxyCommand=%s --proxy %s --target %%h:%%p" USER@HOST`, helper, proxyAddress)
	}
	helper = strings.ReplaceAll(helper, `"`, `\"`)
	return fmt.Sprintf(`ssh -o 'ProxyCommand="%s" --proxy %s --target %%h:%%p' USER@HOST`, helper, proxyAddress)
}

func portRange(minimum, maximum int) string {
	if minimum == maximum {
		return fmt.Sprint(minimum)
	}
	return fmt.Sprintf("%d–%d", minimum, maximum)
}
