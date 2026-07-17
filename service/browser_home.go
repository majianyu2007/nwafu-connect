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
	Monogram    string
	Kind        string
	KindLabel   string
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
<link rel="icon" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'><rect width='24' height='24' rx='6' fill='%230f4233'/><text x='12' y='17' font-family='-apple-system,Segoe UI,sans-serif' font-size='14' font-weight='700' text-anchor='middle' fill='%23ffffff'>N</text></svg>">
<style>
:root {
  color-scheme: light;
  --ink: #15231e;
  --muted: #6a7972;
  --faint: #8d9994;
  --line: #e3ece8;
  --hairline: #edf2f0;
  --paper: #f5f8f6;
  --paper-2: #fbfdfc;
  --card: #ffffff;
  --green: #0f4233;
  --green-2: #1a6049;
  --green-soft: #eaf3ef;
  --green-soft-2: #d8ebdf;
  --shadow: 0 1px 3px rgba(15, 66, 51, .05), 0 6px 18px rgba(15, 66, 51, .04);
  --shadow-hover: 0 2px 4px rgba(15, 66, 51, .06), 0 12px 28px rgba(15, 66, 51, .08);
  --radius: 14px;
  --gold: #c79a3e;
}
* { box-sizing: border-box; }
html { -webkit-font-smoothing: antialiased; -moz-osx-font-smoothing: grayscale; }
body { margin: 0; background: var(--paper); color: var(--ink); font: 14.5px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif; }
button, input { font: inherit; color: inherit; }
.wrap { position: relative; width: min(1200px, calc(100% - 48px)); margin: 0 auto; }
.sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0,0,0,0); white-space: nowrap; }
.hero {
  position: relative; overflow: hidden; color: #f6f9f7;
  background: linear-gradient(140deg, #0c2c23 0%, #163f32 55%, #1d5b46 100%);
}
/* Hero glow done with flat pseudo-elements, no filter: blur, so chrome
   stops re-rasterising large blurred layers on every scroll frame. */
.hero::before { content: ""; position: absolute; pointer-events: none; inset: auto 0 auto auto; width: 50%; height: 100%; background: radial-gradient(closest-side at 80% 80%, rgba(199,154,62,.18), transparent 70%); }
/* Second accent glow kept as a single flat radial; no blur filter. */
.hero::after { content: ""; position: absolute; pointer-events: none; inset: 30% 0 auto auto; width: 36%; height: 60%; background: radial-gradient(closest-side at 30% 50%, rgba(120,200,170,.14), transparent 70%); }
nav { position: relative; display: flex; align-items: center; justify-content: space-between; padding: 24px 0 0; color: #e9f3ee; }
.brand { display: flex; align-items: center; gap: 10px; font-weight: 600; letter-spacing: -.01em; font-size: 15px; }
.mark { display: grid; width: 32px; height: 32px; place-items: center; border-radius: 9px; background: rgba(255,255,255,.12); border: 1px solid rgba(255,255,255,.16); font-size: 15px; font-weight: 700; }
.connected { display: inline-flex; align-items: center; gap: 7px; padding: 5px 11px; border: 1px solid rgba(255,255,255,.14); border-radius: 999px; background: rgba(255,255,255,.06); color: #d6e6dd; font-size: 12px; }
.connected::before { width: 7px; height: 7px; border-radius: 50%; background: #6cd49a; box-shadow: 0 0 0 3px rgba(108,212,154,.18); content: ""; }
.intro { position: relative; display: grid; grid-template-columns: 1fr auto; gap: 40px; align-items: end; padding: 56px 0 70px; }
.eyebrow { margin-bottom: 14px; color: var(--gold); font-size: 11px; font-weight: 600; letter-spacing: .22em; text-transform: uppercase; }
h1 { max-width: 720px; margin: 0; font-size: clamp(28px, 4vw, 42px); line-height: 1.12; letter-spacing: -.02em; font-weight: 600; }
.lead { max-width: 640px; margin: 16px 0 0; color: #b6ccc2; font-size: 14.5px; line-height: 1.65; }
.total { min-width: 132px; padding: 18px 22px; border: 1px solid rgba(255,255,255,.12); border-radius: 14px; background: rgba(255,255,255,.10); }
.total strong { display: block; font-size: 30px; font-weight: 700; line-height: 1; letter-spacing: -.02em; font-variant-numeric: tabular-nums; }
.total span { display: block; margin-top: 6px; color: #b6ccc2; font-size: 12px; }

.content { padding: 0 0 80px; }
.search-panel {
  position: relative; display: flex; gap: 12px; align-items: center;
  margin: -34px auto 0; padding: 14px 18px;
  border: 1px solid rgba(15,66,51,.08); border-radius: 16px;
  background: white; box-shadow: 0 8px 28px rgba(15,66,51,.10);
}
.search-icon { flex: 0 0 auto; color: #7a8983; display: grid; place-items: center; }
#resourceSearch { width: 100%; border: 0; outline: 0; color: var(--ink); background: transparent; font-size: 15px; }
#resourceSearch::placeholder { color: #99a8a1; }
.shortcut { padding: 3px 7px; border: 1px solid var(--line); border-radius: 6px; color: #859390; background: #f4f7f5; font-size: 11px; letter-spacing: .02em; }

.protocol-toggle-wrap { text-align: center; margin-top: 18px; }
.protocol-toggle {
  display: inline-flex; align-items: center; gap: 6px;
  padding: 6px 13px; border: 1px solid var(--line); border-radius: 999px;
  color: var(--green-2); background: white; cursor: pointer;
  font-size: 12px; font-weight: 500;
  transition: background .15s ease, border-color .15s ease;
}
.protocol-toggle:hover { background: var(--green-soft); border-color: var(--green-soft-2); }
.protocol-toggle .chev { transition: transform .18s ease; }
.protocol-toggle[aria-expanded="true"] .chev { transform: rotate(90deg); }
.protocol-guide { display: none; margin: 10px auto 0; padding: 16px 18px; border: 1px solid var(--line); border-radius: 14px; background: var(--paper-2); max-width: 980px; }
.protocol-guide.open { display: block; }
.protocol-guide strong { display: block; margin-bottom: 4px; color: var(--green); font-size: 13px; font-weight: 600; }
.protocol-guide p { margin: 0; color: var(--muted); font-size: 12px; }
.protocol-guide code { display: block; overflow-x: auto; margin-top: 10px; padding: 10px 12px; border-radius: 8px; color: #cfe6dd; background: #133025; font: 12px/1.5 "SF Mono", Menlo, Consolas, monospace; white-space: nowrap; }
.copy-button { margin-top: 10px; padding: 7px 13px; border: 1px solid var(--green-soft-2); border-radius: 8px; color: var(--green); background: white; cursor: pointer; font-size: 12px; font-weight: 500; transition: background .15s ease, border-color .15s ease, color .15s ease; }
.copy-button:hover { background: var(--green-soft); border-color: var(--green-2); }
.copy-button.copied { background: var(--green); color: white; border-color: var(--green); }

.section-head { display: flex; align-items: flex-end; justify-content: space-between; gap: 20px; margin: 48px 0 18px; }
h2 { margin: 0; font-size: 19px; font-weight: 600; letter-spacing: -.01em; }
.section-head p { margin: 5px 0 0; color: var(--muted); font-size: 13px; }
#resultCount { color: var(--faint); font-size: 12px; white-space: nowrap; font-variant-numeric: tabular-nums; }

.grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 14px; }

.card {
  position: relative; min-width: 0; overflow: hidden;
  border: 1px solid var(--line); border-radius: var(--radius); background: var(--card);
  transition: transform .16s ease, border-color .16s ease, box-shadow .16s ease;
  animation: cardIn .28s ease backwards;
  content-visibility: auto;
  contain-intrinsic-size: 200px;
}
.card.has-additional { cursor: pointer; }
.card.has-url { cursor: pointer; }
.card.has-url:focus-visible { outline: 0; border-color: var(--green-2); box-shadow: 0 0 0 3px rgba(26,96,73,.18); }
.card.has-url:hover { transform: translateY(-1px); border-color: #c5d6cf; box-shadow: var(--shadow-hover); }
.card.has-url:active { transform: translateY(0); box-shadow: var(--shadow); }
.card-main { padding: 16px 16px 14px; }
.card-row { display: flex; gap: 12px; align-items: flex-start; }
@keyframes cardIn { from { opacity: 0; transform: translateY(3px); } to { opacity: 1; transform: none; } }
.resource-icon {
  display: grid; flex: 0 0 auto; width: 40px; height: 40px; place-items: center; border-radius: 11px;
  background: linear-gradient(140deg, #eef5f1 0%, #d9ecdf 100%);
  color: var(--green); font-size: 15px; font-weight: 700; letter-spacing: -.01em;
  box-shadow: inset 0 0 0 1px rgba(15,66,51,.04);
}
.card-copy { min-width: 0; flex: 1; }
.resource-name { display: block; overflow: hidden; font-weight: 600; font-size: 14.5px; letter-spacing: -.005em; text-overflow: ellipsis; white-space: nowrap; }
.type-badge { display: inline-flex; align-items: center; gap: 5px; margin-top: 6px; padding: 2px 8px; border-radius: 999px; font-size: 10.5px; font-weight: 500; letter-spacing: .04em; background: #eef2f0; color: var(--muted); text-transform: uppercase; }
.type-badge .dot { width: 6px; height: 6px; border-radius: 50%; background: currentColor; }
.type-badge.kind-web { color: #1a6049; background: #e6f1ea; }
.type-badge.kind-ssh { color: #b86a23; background: #f8eede; }
.type-badge.kind-tcp { color: #5a6770; background: #edf1f3; }
.type-badge.kind-all { color: #6246c2; background: #ecedf8; }
.type-badge.kind-mix { color: #1a6049; background: #e6f1ea; }
.description { display: -webkit-box; overflow: hidden; margin-top: 6px; color: var(--muted); font-size: 12.5px; line-height: 1.55; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }

.address, .address-static { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 10px; align-items: center; padding: 9px 16px; border-top: 1px solid var(--hairline); color: var(--green-2); background: var(--paper-2); font-size: 12px; }
.address { cursor: pointer; transition: background .14s ease; }
.address:hover { background: #ecf3ee; }
.address[data-url]:focus-visible { outline: 0; background: #e3efea; }
.host { overflow: hidden; font-weight: 500; text-overflow: ellipsis; white-space: nowrap; font-feature-settings: "tnum" 1; }
.meta { color: var(--faint); white-space: nowrap; font-variant-numeric: tabular-nums; }

.address-hint { cursor: pointer; }
.address-hint:hover { background: #ecf3ee; }
.address-hint .meta { color: var(--green-2); font-weight: 500; }
details { border-top: 1px solid var(--hairline); background: var(--paper-2); }
summary { padding: 9px 16px; color: var(--muted); cursor: pointer; font-size: 12px; list-style: none; display: flex; align-items: center; gap: 6px; }
summary::-webkit-details-marker { display: none; }
summary::before { content: ""; width: 0; height: 0; border: 4px solid transparent; border-left-color: currentColor; opacity: .6; }
details[open] summary::before { transform: rotate(90deg); }
details .address, details .address-static { padding-left: 28px; border-top-color: #f0f4f2; }

.card.has-url .open-chev { position: absolute; top: 12px; right: 14px; width: 16px; height: 16px; color: #b3c5bc; opacity: 0; transition: opacity .18s ease, transform .18s ease; }
.card.has-url:hover .open-chev, .card.has-url:focus-visible .open-chev { opacity: 1; transform: translate(2px, -2px); }

.empty { display: none; padding: 56px 24px; border: 1px dashed var(--line); border-radius: var(--radius); color: var(--muted); text-align: center; background: var(--paper-2); }
.empty-icon { display: block; margin: 0 auto 12px; color: #b7c7bf; }
.empty strong { display: block; margin-bottom: 4px; color: var(--ink); font-size: 15px; font-weight: 600; }

.actions { display: flex; justify-content: center; margin-top: 26px; }
#showMore { padding: 9px 18px; border: 1px solid var(--line); border-radius: 10px; color: var(--green); background: white; cursor: pointer; font-weight: 500; font-size: 13px; transition: background .15s ease, border-color .15s ease; }
#showMore:hover { background: var(--green-soft); border-color: var(--green-soft-2); }

.notice { display: grid; grid-template-columns: auto 1fr; gap: 14px; margin-top: 44px; padding: 18px 20px; border: 1px solid var(--line); border-radius: var(--radius); background: var(--paper-2); }
.notice-icon { display: grid; width: 32px; height: 32px; place-items: center; border-radius: 9px; color: var(--green-2); background: var(--green-soft); }
.notice strong { display: block; margin-bottom: 3px; font-size: 13.5px; font-weight: 600; }
.notice p { margin: 0; color: var(--muted); font-size: 13px; line-height: 1.6; }

footer { margin-top: 44px; padding-bottom: 8px; color: var(--faint); font-size: 12px; text-align: center; }

@media (max-width: 850px) {
  .intro { grid-template-columns: 1fr; gap: 24px; padding-top: 36px; }
  .total { width: fit-content; }
}
@media (max-width: 560px) {
  .wrap { width: min(100% - 28px, 1200px); }
  nav { padding-top: 18px; }
  .connected { font-size: 0; }
  .connected::after { content: "已连接"; font-size: 12px; }
  .intro { padding-bottom: 72px; }
  .shortcut { display: none; }
  .section-head { align-items: flex-start; flex-direction: column; gap: 6px; }
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
        <p class="lead">浏览本次登录由学校 aTrust 网关授权的资源。点击资源，NWAFU Connect 会自动通过对应安全隧道访问。</p>
      </div>
      <div class="total"><strong>{{len .Resources}}</strong><span>项应用资源</span></div>
    </div>
  </div>
</header>
<main class="content wrap">
  <label class="search-panel" for="resourceSearch">
    <span class="search-icon" aria-hidden="true"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/></svg></span>
    <span class="sr-only">搜索资源名称、说明或地址</span>
    <input id="resourceSearch" type="search" placeholder="搜索资源名称、说明或地址…" autocomplete="off">
    <span class="shortcut">⌘ K</span>
  </label>
  <div class="protocol-toggle-wrap">
    <button class="protocol-toggle" id="sshToggle" type="button" aria-expanded="false" aria-controls="sshGuide">
      <svg class="chev" width="10" height="10" viewBox="0 0 12 12" fill="currentColor" aria-hidden="true"><path d="M3.5 2 8 6 3.5 10"/></svg>
      SSH / SFTP 等 TCP 客户端接入
    </button>
  </div>
  <section class="protocol-guide" id="sshGuide" role="region" aria-labelledby="sshToggle">
    <strong>SSH / SFTP 等 TCP 客户端</strong>
    <p>使用随应用打包的 stdio 代理助手，将任意 TCP 客户端接入当前 aTrust 会话：</p>
    <code id="sshCommand">{{.SSHCommand}}</code>
    <button class="copy-button" type="button" data-copy="sshCommand">复制命令</button>
  </section>
  <div class="section-head">
    <div><h2>全部资源</h2><p>资源权限由学校 aTrust 网关实时下发</p></div>
    <span id="resultCount">{{len .Resources}} 个结果</span>
  </div>
  <section id="resourceGrid" class="grid" aria-live="polite">
    {{range $i, $r := .Resources}}<article class="card{{if $r.Primary.URL}} has-url{{end}}{{if $r.Additional}} has-additional{{end}}" data-search="{{$r.SearchText}}" style="animation-delay: {{$i}}ms;"{{if $r.Primary.URL}} data-url="{{$r.Primary.URL}}" tabindex="0" role="link"{{end}}><div class="card-main"><div class="card-row"><span class="resource-icon" aria-hidden="true">{{$r.Monogram}}</span><div class="card-copy"><span class="resource-name">{{$r.Name}}</span>{{if $r.Kind}}<span class="type-badge kind-{{$r.Kind}}"><span class="dot"></span>{{$r.KindLabel}}</span>{{end}}{{if $r.Description}}<span class="description">{{$r.Description}}</span>{{end}}</div></div></div>{{if $r.Primary.URL}}<div class="address" data-url="{{$r.Primary.URL}}" role="link" tabindex="0"><span class="host">{{$r.Primary.Host}}</span><span class="meta">{{$r.Primary.Protocol}} · {{$r.Primary.Ports}}</span></div>{{else}}{{if $r.Additional}}<div class="address-static address-hint"><span class="host">{{$r.Primary.Host}}</span><span class="meta">点击展开 ↓</span></div>{{else}}<div class="address-static"><span class="host">{{$r.Primary.Host}}</span><span class="meta">{{$r.Primary.Protocol}} · {{$r.Primary.Ports}}</span></div>{{end}}{{end}}{{if $r.Additional}}<details><summary>另外 {{len $r.Additional}} 个下发地址</summary>{{range $r.Additional}}{{if .URL}}<div class="address" data-url="{{.URL}}" role="link" tabindex="0"><span class="host">{{.Host}}</span><span class="meta">{{.Protocol}} · {{.Ports}}</span></div>{{else}}<div class="address-static"><span class="host">{{.Host}}</span><span class="meta">{{.Protocol}} · {{.Ports}}</span></div>{{end}}{{end}}</details>{{end}}{{if $r.Primary.URL}}<svg class="open-chev" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 11 11 5"/><path d="M6 5h5v5"/></svg>{{end}}</article>{{end}}
  </section>
  <div id="emptyState" class="empty">
    <svg class="empty-icon" width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="11" cy="11" r="7"/><path d="m21 21-4.3-4.3"/></svg>
    <strong>没有找到匹配的资源</strong>
    请尝试其他关键词，或清除搜索查看全部下发资源。
  </div>
  <div class="actions"><button id="showMore" type="button">显示全部资源</button></div>
  <aside class="notice">
    <span class="notice-icon" aria-hidden="true"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/></svg></span>
    <div><strong>找不到需要的校内网站？</strong><p>NWAFU Connect 只能使用学校 aTrust 网关下发并授权的资源。若校内网站未出现在列表中，需要由学校网络管理员将它加入 aTrust 资源策略，客户端无法绕过网关权限。</p></div>
  </aside>
  <footer>NWAFU Connect · 临时受管浏览器会话</footer>
</main>
<script>
(() => {
  const input = document.getElementById("resourceSearch");
  const cards = Array.from(document.querySelectorAll(".card"));
  const count = document.getElementById("resultCount");
  const empty = document.getElementById("emptyState");
  function render() {
    const query = input.value.trim().toLowerCase();
    const matches = query ? cards.filter(card => card.dataset.search.toLowerCase().includes(query)) : cards;
    const total = matches.length;
    const visible = query || expanded ? matches : matches.slice(0, initialLimit);
    const visibleSet = new Set(visible);
    for (const card of cards) {
      const shouldShow = visibleSet.has(card);
      if (card.hidden !== !shouldShow) card.hidden = !shouldShow;
    }
    count.textContent = total + " 个结果";
    empty.style.display = total ? "none" : "block";
    more.style.display = !query && cards.length > initialLimit ? "inline-block" : "none";
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
    const original = button.textContent;
    button.addEventListener("click", async (event) => {
      event.stopPropagation();
      const text = document.getElementById(button.dataset.copy).textContent;
      try { await navigator.clipboard.writeText(text); }
      catch (_) {
        const area = document.createElement("textarea");
        area.value = text;
        document.body.appendChild(area);
        area.select();
        document.execCommand("copy");
        area.remove();
      }
      button.classList.add("copied");
      button.textContent = "已复制";
      setTimeout(() => {
        button.classList.remove("copied");
        button.textContent = original;
      }, 1500);
    });
  });
  const sshToggle = document.getElementById("sshToggle");
  const sshGuide = document.getElementById("sshGuide");
  if (sshToggle && sshGuide) {
    sshToggle.addEventListener("click", () => {
      const open = sshGuide.classList.toggle("open");
      sshToggle.setAttribute("aria-expanded", open ? "true" : "false");
    });
  }
  function openResource(url) {
    if (!url) return;
    const toast = document.createElement("div");
    toast.textContent = "正在通过安全隧道打开…";
    toast.style.cssText = "position:fixed;bottom:24px;left:50%;transform:translateX(-50%);padding:10px 18px;border-radius:10px;background:#0f4233;color:#fff;font-size:13px;letter-spacing:.02em;box-shadow:0 8px 24px rgba(15,66,51,.28);z-index:9999;opacity:0;transition:opacity .18s ease;";
    document.body.appendChild(toast);
    requestAnimationFrame(() => { toast.style.opacity = "1"; });
    const a = document.createElement("a");
    a.href = url; a.target = "_blank"; a.rel = "noopener noreferrer";
    document.body.appendChild(a);
    a.click(); a.remove();
    setTimeout(() => { toast.style.opacity = "0"; setTimeout(() => toast.remove(), 220); }, 900);
  }
  document.querySelectorAll(".card").forEach(card => {
    const details = card.querySelector("details");
    card.addEventListener("click", (event) => {
      if (event.target.closest("details") || event.target.closest("summary")) return;
      if (event.target.closest(".address")) return;
      if (card.dataset.url) { event.preventDefault(); openResource(card.dataset.url); return; }
      // Range-only primary host: reveal the additional issued addresses so the
      // user can reach the discrete URLs the gateway actually authorized.
      if (details) { details.open = !details.open; }
    });
    card.addEventListener("keydown", (event) => {
      if (event.key !== "Enter" && event.key !== " ") return;
      if (card.dataset.url) { event.preventDefault(); openResource(card.dataset.url); }
      else if (details) { event.preventDefault(); details.open = !details.open; }
    });
  });
  document.querySelectorAll(".address[data-url]").forEach(row => {
    row.addEventListener("click", (event) => {
      event.preventDefault(); event.stopPropagation();
      openResource(row.dataset.url);
    });
    row.addEventListener("keydown", (event) => {
      if ((event.key === "Enter" || event.key === " ") && row.dataset.url) {
        event.preventDefault(); openResource(row.dataset.url);
      }
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
		kind, kindLabel := inferResourceType(source)
		resources = append(resources, browserHomeResource{
			Name:        name,
			Description: strings.TrimSpace(source.Description),
			SearchText:  strings.Join(searchParts, " "),
			Monogram:    resourceMonogram(name),
			Kind:        kind,
			KindLabel:   kindLabel,
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
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src data:; style-src 'unsafe-inline'; script-src 'unsafe-inline'")
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

// resourceMonogram derives a short (1-2 character) badge label shown inside
// the resource icon. The full resource name is still displayed as the card
// title; this is only the 2-glyph badge. It takes the first two CJK runes of
// the name, falls back to uppercase ASCII initials for latin/domain-like
// names, and collapses pure IP addresses to "IP".
func resourceMonogram(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "·"
	}
	if net.ParseIP(name) != nil {
		return "IP"
	}
	var cjk []rune
	for _, r := range name {
		if r >= 0x4E00 && r <= 0x9FFF {
			cjk = append(cjk, r)
			if len(cjk) == 2 {
				return string(cjk)
			}
		}
	}
	if len(cjk) == 1 {
		return string(cjk)
	}
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == ' ' || r == '.' || r == '-' || r == '_' || r == '/' || r == '@' || r == ':'
	})
	if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
		return strings.ToUpper(string(parts[0][0]) + string(parts[1][0]))
	}
	runes := []rune(name)
	if len(runes) >= 2 {
		return strings.ToUpper(string(runes[:2]))
	}
	return strings.ToUpper(string(runes))
}

// inferResourceType classifies a resource by the ports its addresses expose so
// the portal can show a small "Web / SSH / TCP / 全部 / 混合" badge on each card.
func inferResourceType(source client.Resource) (kind, label string) {
	hasWeb, hasSSH, hasWide := false, false, false
	for _, address := range source.Addresses {
		if address.Protocol != "tcp" && address.Protocol != "all" {
			continue
		}
		if address.PortMin <= 22 && address.PortMax >= 22 {
			hasSSH = true
		}
		if (address.PortMin <= 80 && address.PortMax >= 80) || (address.PortMin <= 443 && address.PortMax >= 443) {
			hasWeb = true
		}
		if address.Protocol == "all" || (address.PortMin <= 1 && address.PortMax >= 65535) {
			hasWide = true
		}
	}
	switch {
	case (hasSSH && hasWeb) || (hasWide && (hasSSH || hasWeb)):
		return "mix", "混合"
	case hasSSH:
		return "ssh", "SSH"
	case hasWeb:
		return "web", "Web"
	case hasWide:
		return "all", "全部"
	default:
		return "tcp", "TCP"
	}
}
