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
	Resources       []browserHomeResource
	SSHCommand      string
	SSHCommandShell string
	HasMore         bool
	InitialLimit    int
	Total           int
}

var browserHomeTemplate = template.Must(template.New("browser-home").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>NWAFU Connect · 校内资源</title>
<link rel="icon" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'><rect width='24' height='24' rx='6' fill='%230d4f3c'/><text x='12' y='17' font-family='-apple-system,Segoe UI,sans-serif' font-size='14' font-weight='700' text-anchor='middle' fill='%23ffffff'>N</text></svg>">
<style>
:root {
  color-scheme: light;
  --brand: #0d4f3c;
  --brand-strong: #083b2d;
  --brand-soft: #e5f0eb;
  --brand-faint: #f1f7f4;
  --ink: #17251f;
  --muted: #5f7068;
  --quiet: #576861;
  --line: #dbe6e1;
  --line-soft: #eaf0ed;
  --canvas: #f2f6f4;
  --surface: #ffffff;
  --radius-surface: 16px;
  --radius-control: 10px;
  --shadow-toolbar: 0 10px 30px rgba(8, 59, 45, .10);
  --shadow-card: 0 8px 24px rgba(8, 59, 45, .06);
}
* { box-sizing: border-box; }
[hidden] { display: none !important; }
html { -webkit-font-smoothing: antialiased; -moz-osx-font-smoothing: grayscale; }
body {
  margin: 0;
  background: var(--canvas);
  color: var(--ink);
  font: 15px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif;
}
button, input { color: inherit; font: inherit; }
button, a { touch-action: manipulation; }
a { color: inherit; }
.wrap { width: min(1120px, calc(100% - 40px)); margin: 0 auto; }
.skip-link {
  position: fixed; z-index: 1000; top: 8px; left: 8px; padding: 8px 12px;
  border-radius: 8px; background: white; color: var(--brand); transform: translateY(-150%);
}
.skip-link:focus { transform: none; }

.site-header { background: var(--brand-strong); color: white; }
.site-header .wrap {
  display: flex; min-height: 64px; align-items: center; justify-content: space-between; gap: 20px;
}
.brand { display: flex; align-items: center; gap: 11px; font-size: 15px; font-weight: 650; letter-spacing: -.01em; }
.brand-mark {
  display: grid; width: 34px; height: 34px; place-items: center;
  border: 1px solid rgba(255,255,255,.22); border-radius: 10px;
  background: rgba(255,255,255,.10); font-weight: 750;
}
.connection-state { display: inline-flex; align-items: center; gap: 8px; color: #d9e8e1; font-size: 13px; }
.status-dot { width: 8px; height: 8px; border-radius: 50%; background: #72d49e; box-shadow: 0 0 0 3px rgba(114,212,158,.16); }

main { padding-bottom: 64px; }
.portal-intro {
  display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: end; gap: 40px;
  padding: 38px 0 30px;
}
.session-label { margin: 0 0 6px; color: var(--brand); font-size: 13px; font-weight: 650; }
h1 { margin: 0; font-size: clamp(30px, 4vw, 43px); line-height: 1.14; letter-spacing: -.035em; font-weight: 680; }
.intro-copy { max-width: 660px; margin: 12px 0 0; color: var(--muted); font-size: 15px; }
.resource-total { min-width: 128px; padding-left: 24px; border-left: 1px solid var(--line); }
.resource-total strong { display: block; color: var(--brand); font-size: 34px; line-height: 1; font-variant-numeric: tabular-nums; }
.resource-total span { display: block; margin-top: 7px; color: var(--muted); font-size: 13px; }

.discovery-tools {
  position: sticky; z-index: 20; top: 12px;
  display: grid; grid-template-columns: minmax(260px, 1fr) auto; gap: 16px; align-items: end;
  padding: 14px; border: 1px solid var(--line); border-radius: var(--radius-surface);
  background: rgba(255,255,255,.98); box-shadow: var(--shadow-toolbar);
}
.search-group label, .filter-label {
  display: block; margin: 0 0 6px 2px; color: var(--muted); font-size: 12px; font-weight: 600;
}
.search-control {
  display: flex; min-height: 44px; align-items: center; gap: 10px;
  padding: 0 12px; border: 1px solid #cddbd5; border-radius: var(--radius-control); background: white;
}
.search-control:focus-within { border-color: var(--brand); box-shadow: 0 0 0 3px rgba(13,79,60,.12); }
.search-icon { display: grid; flex: 0 0 auto; color: var(--quiet); }
#resourceSearch { width: 100%; min-width: 0; min-height: 42px; border: 0; outline: 0; background: transparent; font-size: 16px; }
#resourceSearch::placeholder { color: #87958f; }
.shortcut {
  flex: 0 0 auto; padding: 2px 7px; border: 1px solid var(--line);
  border-radius: 6px; background: var(--canvas); color: var(--quiet); font-size: 11px; white-space: nowrap;
}
.filters { display: flex; gap: 6px; }
.filter-button {
  min-height: 44px; padding: 0 13px; border: 1px solid var(--line); border-radius: 999px;
  background: white; color: var(--muted); cursor: pointer; font-size: 13px; font-weight: 550;
}
.filter-button:hover { border-color: #b9cec5; background: var(--brand-faint); color: var(--brand); }
.filter-button[aria-pressed="true"] { border-color: var(--brand); background: var(--brand); color: white; }
.filter-button:focus-visible, .secondary-button:focus-visible, .copy-button:focus-visible, summary:focus-visible {
  outline: 3px solid #2e705c; outline-offset: 2px;
}

.resource-section { margin-top: 34px; }
.section-heading { display: flex; align-items: end; justify-content: space-between; gap: 20px; margin-bottom: 14px; }
.section-heading h2 { margin: 0; font-size: 20px; line-height: 1.3; letter-spacing: -.015em; }
.section-heading p { margin: 5px 0 0; color: var(--muted); font-size: 13px; }
#resultCount { color: var(--quiet); font-size: 13px; font-variant-numeric: tabular-nums; white-space: nowrap; }
.resource-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 14px; }
.resource-card {
  display: flex; min-width: 0; flex-direction: column; overflow: hidden;
  border: 1px solid var(--line); border-radius: var(--radius-surface); background: var(--surface);
  transition: border-color .16s ease, box-shadow .16s ease;
}
.resource-card:hover { border-color: #bfd1c9; box-shadow: var(--shadow-card); }
.card-body { display: flex; min-height: 128px; flex: 1; flex-direction: column; padding: 17px 17px 14px; }
.card-heading { display: flex; gap: 12px; align-items: flex-start; }
.resource-mark {
  display: grid; flex: 0 0 auto; width: 42px; height: 42px; place-items: center;
  border-radius: 12px; background: var(--brand-soft); color: var(--brand); font-size: 14px; font-weight: 720;
}
.title-group { min-width: 0; }
.resource-name { margin: 0; overflow-wrap: anywhere; font-size: 15px; line-height: 1.42; font-weight: 650; letter-spacing: -.005em; }
.type-badge {
  display: inline-flex; margin-top: 6px; padding: 2px 8px; border-radius: 999px;
  background: #edf2f0; color: var(--muted); font-size: 10.5px; font-weight: 650; letter-spacing: .04em;
}
.type-badge.kind-web, .type-badge.kind-all, .type-badge.kind-mix { background: var(--brand-soft); color: var(--brand); }
.description {
  display: -webkit-box; overflow: hidden; margin: 10px 0 0; color: var(--muted);
  font-size: 13px; line-height: 1.55; -webkit-box-orient: vertical; -webkit-line-clamp: 2;
}
.address-list { border-top: 1px solid var(--line-soft); background: #fbfdfc; }
.resource-link, .address-static {
  display: grid; min-height: 48px; grid-template-columns: minmax(0, 1fr) auto;
  gap: 12px; align-items: center; padding: 9px 16px; text-decoration: none;
}
.resource-link > span:first-child { display: flex; min-width: 0; align-items: baseline; gap: 5px; }
.resource-link { color: var(--brand); }
.resource-link:hover { background: var(--brand-faint); }
.resource-link:focus-visible { outline: 3px solid #2e705c; outline-offset: -3px; }
.address-host { min-width: 0; overflow: hidden; font-size: 12.5px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.address-meta { color: var(--quiet); font-size: 11px; text-transform: uppercase; white-space: nowrap; }
.open-action { color: var(--brand); font-size: 12px; font-weight: 650; white-space: nowrap; }
.address-static .address-host { color: var(--muted); }
details.addresses { border-top: 1px solid var(--line-soft); }
details.addresses summary {
  display: flex; min-height: 44px; align-items: center; justify-content: space-between; gap: 12px;
  padding: 8px 16px; color: var(--muted); cursor: pointer; font-size: 12px; list-style: none;
}
details.addresses summary::-webkit-details-marker { display: none; }
details.addresses summary::after { content: "展开"; color: var(--brand); font-weight: 600; }
details.addresses[open] summary::after { content: "收起"; }
details.addresses .resource-link, details.addresses .address-static { border-top: 1px solid var(--line-soft); padding-left: 22px; }

.empty-state {
  padding: 56px 24px; border: 1px dashed #c6d7d0; border-radius: var(--radius-surface);
  background: rgba(255,255,255,.62); color: var(--muted); text-align: center;
}
.empty-state strong { display: block; margin-bottom: 5px; color: var(--ink); font-size: 16px; }
.secondary-button, .copy-button {
  min-height: 44px; padding: 0 15px; border: 1px solid #c7d8d0; border-radius: var(--radius-control);
  background: white; color: var(--brand); cursor: pointer; font-size: 13px; font-weight: 600;
}
.secondary-button:hover, .copy-button:hover { border-color: var(--brand); background: var(--brand-faint); }
.empty-state .secondary-button { margin-top: 16px; }
.list-actions { display: flex; justify-content: center; margin-top: 22px; }

.help-grid { display: grid; grid-template-columns: minmax(0, 1fr); gap: 12px; margin-top: 36px; }
.notice, .client-access {
  border: 1px solid var(--line); border-radius: var(--radius-surface); background: rgba(255,255,255,.72);
}
.notice { padding: 17px 18px; }
.notice strong { display: block; margin-bottom: 3px; font-size: 14px; }
.notice p { margin: 0; color: var(--muted); font-size: 13px; }
.client-access summary {
  display: grid; min-height: 54px; grid-template-columns: minmax(0, 1fr) auto; align-items: center;
  column-gap: 20px; row-gap: 2px; padding: 10px 18px; cursor: pointer; list-style: none;
}
.client-access summary::-webkit-details-marker { display: none; }
.client-access summary strong { grid-column: 1; grid-row: 1; font-size: 14px; }
.client-access summary > span { grid-column: 1; grid-row: 2; color: var(--quiet); font-size: 12px; }
.client-access summary::after {
  content: "展开 ↓"; grid-column: 2; grid-row: 1 / span 2;
  color: var(--brand); font-size: 12px; font-weight: 650; white-space: nowrap;
}
.client-access[open] summary::after { content: "收起 ↑"; }
.client-guide { padding: 0 18px 18px; border-top: 1px solid var(--line-soft); }
.client-guide p { margin: 14px 0 0; color: var(--muted); font-size: 13px; }
.client-guide code {
  display: block; overflow-x: auto; margin-top: 12px; padding: 12px 13px;
  border-radius: var(--radius-control); background: #112d24; color: #dcece5;
  font: 12px/1.55 "SF Mono", Menlo, Consolas, monospace; white-space: nowrap;
}
.copy-button { margin-top: 12px; }
.copy-button.copied { border-color: var(--brand); background: var(--brand); color: white; }
footer { margin-top: 36px; color: var(--quiet); font-size: 12px; text-align: center; }

@media (max-width: 920px) {
  .resource-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .discovery-tools { grid-template-columns: 1fr; align-items: stretch; }
  .filters { overflow-x: auto; padding-bottom: 2px; }
}
@media (max-width: 640px) {
  .wrap { width: min(100% - 28px, 1120px); }
  .site-header .wrap { min-height: 58px; }
  .connection-state span:last-child { font-size: 0; }
  .connection-state span:last-child::after { content: "已连接"; font-size: 12px; }
  .portal-intro { grid-template-columns: 1fr; gap: 20px; padding: 28px 0 24px; }
  .resource-total { display: flex; min-width: 0; align-items: baseline; gap: 8px; padding: 0; border: 0; }
  .resource-total strong { font-size: 26px; }
  .resource-total span { margin: 0; }
  .discovery-tools { position: static; padding: 12px; }
  .filter-button { min-height: 44px; }
  .resource-section { margin-top: 28px; }
  .section-heading { align-items: flex-start; flex-direction: column; gap: 6px; }
  .resource-grid { grid-template-columns: 1fr; }
  .card-body { min-height: 0; }
  .shortcut { display: none; }
  .client-access summary { column-gap: 12px; }
}
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { scroll-behavior: auto !important; transition-duration: .01ms !important; }
}
</style>
</head>
<body>
<a class="skip-link" href="#resourceSearch">跳到资源搜索</a>
<header class="site-header">
  <div class="wrap">
    <div class="brand"><span class="brand-mark" aria-hidden="true">N</span><span>NWAFU Connect</span></div>
    <div class="connection-state"><span class="status-dot" aria-hidden="true"></span><span>aTrust 安全连接已建立</span></div>
  </div>
</header>
<main class="wrap">
  <section class="portal-intro" aria-labelledby="pageTitle">
    <div>
      <p class="session-label">本次会话已授权</p>
      <h1 id="pageTitle">校内资源</h1>
      <p class="intro-copy">这里仅显示学校 aTrust 网关下发的访问权限。打开资源后，请保持 NWAFU Connect 和此受管浏览器运行。</p>
    </div>
    <div class="resource-total" aria-label="共 {{.Total}} 项资源"><strong>{{.Total}}</strong><span>项可用资源</span></div>
  </section>

  <section class="discovery-tools" aria-label="资源查找工具">
    <div class="search-group">
      <label for="resourceSearch">查找资源</label>
      <div class="search-control">
        <span class="search-icon" aria-hidden="true"><svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"><circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/></svg></span>
        <input id="resourceSearch" type="search" placeholder="输入名称、说明或地址" autocomplete="off" spellcheck="false">
        <span class="shortcut" id="searchShortcut">⌘ K</span>
      </div>
    </div>
    <div>
      <span class="filter-label">资源类型</span>
      <div class="filters" role="group" aria-label="按资源类型筛选">
        <button class="filter-button" type="button" data-kind="all" aria-pressed="true">全部</button>
        <button class="filter-button" type="button" data-kind="web" aria-pressed="false">网页</button>
        <button class="filter-button" type="button" data-kind="ssh" aria-pressed="false">SSH</button>
        <button class="filter-button" type="button" data-kind="client" aria-pressed="false">客户端</button>
      </div>
    </div>
  </section>

  <section class="resource-section" aria-labelledby="resourceHeading">
    <div class="section-heading">
      <div><h2 id="resourceHeading">资源列表</h2><p>选择明确的地址打开；非网页资源会保留连接信息供客户端使用。</p></div>
      <span id="resultCount" aria-live="polite" aria-atomic="true">{{if .HasMore}}已显示 {{len .Resources}} / {{.Total}} 项{{else}}{{len .Resources}} 项{{end}}</span>
    </div>
    <div id="resourceGrid" class="resource-grid">
      {{range $resource := .Resources}}{{template "resource-card" $resource}}{{end}}
    </div>
    <div id="emptyState" class="empty-state"{{if .Resources}} hidden{{end}}>
      <strong>没有匹配的资源</strong>
      <span>换一个关键词，或清除类型筛选后再试。</span>
      <div><button class="secondary-button" id="clearFilters" type="button">清除筛选</button></div>
    </div>
    {{if .HasMore}}<div class="list-actions"><button class="secondary-button" id="showMore" type="button">显示更多资源</button></div>{{end}}
  </section>

  <div class="help-grid">
    <aside class="notice">
      <strong>列表中没有需要的网站？</strong>
      <p>客户端不能绕过学校网关权限。请联系学校网络管理员，将目标加入你的 aTrust 资源策略。</p>
    </aside>
    <details class="client-access">
      <summary><strong>SSH / SFTP 等客户端如何接入</strong><span>高级用法，普通网页访问无需设置</span></summary>
      <div class="client-guide">
        <p>使用随应用提供的 stdio 代理助手，把 TCP 客户端接入当前会话；请复制到 {{.SSHCommandShell}} 运行：</p>
        <code id="sshCommand">{{.SSHCommand}}</code>
        <button class="copy-button" type="button" data-copy="sshCommand">复制命令</button>
      </div>
    </details>
  </div>
  <footer>临时受管浏览器会话 · 关闭连接后资源将不可访问</footer>
</main>
<script>
(() => {
  const initialLimit = {{.InitialLimit}};
  const totalResources = {{.Total}};
  const input = document.getElementById("resourceSearch");
  const grid = document.getElementById("resourceGrid");
  let cards = Array.from(document.querySelectorAll(".resource-card"));
  const count = document.getElementById("resultCount");
  const empty = document.getElementById("emptyState");
  const clear = document.getElementById("clearFilters");
  const more = document.getElementById("showMore");
  const filterButtons = Array.from(document.querySelectorAll(".filter-button"));
  const shortcut = document.getElementById("searchShortcut");
  let activeKind = "all";
  let expanded = false;
  let renderPending = false;
  let deferredLoaded = !more;
  let hydrationPromise = null;

  if (!/Mac|iPhone|iPad/.test(navigator.platform)) shortcut.textContent = "Ctrl K";

  async function hydrateDeferred() {
    if (deferredLoaded) return true;
    if (hydrationPromise) return hydrationPromise;
    more.hidden = false;
    more.disabled = true;
    more.textContent = "正在加载资源…";
    hydrationPromise = (async () => {
      const response = await fetch("/resources", {headers: {"Accept": "text/html"}});
      if (!response.ok) throw new Error("HTTP " + response.status);
      const markup = await response.text();
      const fragment = document.createElement("template");
      fragment.innerHTML = markup;
      grid.appendChild(fragment.content);
      cards = Array.from(document.querySelectorAll(".resource-card"));
      deferredLoaded = true;
      return true;
    })();
    try {
      return await hydrationPromise;
    } catch (_) {
      count.textContent = "资源加载失败";
      more.disabled = false;
      more.textContent = "加载失败，点击重试";
      return false;
    } finally {
      hydrationPromise = null;
    }
  }

  function render() {
    renderPending = false;
    const query = input.value.trim().toLocaleLowerCase();
    let matches = 0;
    let visible = 0;
    for (const card of cards) {
      const kind = card.dataset.kind;
      const kindMatches = activeKind === "all" ||
        kind === activeKind ||
        (activeKind === "web" && (kind === "mix" || kind === "all")) ||
        (activeKind === "ssh" && (kind === "mix" || kind === "all")) ||
        (activeKind === "client" && kind !== "web");
      const queryMatches = !query || card.dataset.search.includes(query);
      const matchesCard = kindMatches && queryMatches;
      if (matchesCard) matches++;
      const shouldShow = matchesCard && (expanded || Boolean(query) || matches <= initialLimit);
      card.hidden = !shouldShow;
      if (shouldShow) visible++;
    }
    if (!deferredLoaded && !query && activeKind === "all") {
      count.textContent = "已显示 " + visible + " / " + totalResources + " 项";
    } else {
      count.textContent = visible === matches ? matches + " 项" : "已显示 " + visible + " / " + matches + " 项";
    }
    empty.hidden = matches !== 0;
    if (more) {
      more.disabled = false;
      more.hidden = deferredLoaded && (Boolean(query) || matches <= initialLimit);
      more.textContent = expanded ? "收起资源" : "显示更多资源";
    }
  }

  function scheduleRender() {
    if (renderPending) return;
    renderPending = true;
    requestAnimationFrame(render);
  }

  input.addEventListener("input", async () => {
    if (input.value.trim() && !(await hydrateDeferred())) return;
    scheduleRender();
  });
  filterButtons.forEach(button => {
    button.addEventListener("click", async () => {
      activeKind = button.dataset.kind;
      expanded = false;
      filterButtons.forEach(item => item.setAttribute("aria-pressed", item === button ? "true" : "false"));
      if (!(await hydrateDeferred())) return;
      scheduleRender();
    });
  });
  if (more) {
    more.addEventListener("click", async () => {
      if (!(await hydrateDeferred())) return;
      expanded = !expanded;
      scheduleRender();
    });
  }
  clear.addEventListener("click", () => {
    input.value = "";
    activeKind = "all";
    expanded = false;
    filterButtons.forEach(button => button.setAttribute("aria-pressed", button.dataset.kind === "all" ? "true" : "false"));
    input.focus();
    scheduleRender();
  });
  document.addEventListener("keydown", event => {
    if ((event.metaKey || event.ctrlKey) && event.key.toLocaleLowerCase() === "k") {
      event.preventDefault();
      input.focus();
      input.select();
    } else if (event.key === "Escape" && document.activeElement === input && input.value) {
      input.value = "";
      scheduleRender();
    }
  });

  document.querySelectorAll("[data-copy]").forEach(button => {
    const original = button.textContent;
    button.addEventListener("click", async () => {
      const source = document.getElementById(button.dataset.copy);
      const text = source ? source.textContent : "";
      try {
        await navigator.clipboard.writeText(text);
      } catch (_) {
        const area = document.createElement("textarea");
        area.value = text;
        area.style.position = "fixed";
        area.style.opacity = "0";
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
      }, 1600);
    });
  });
  render();
})();
</script>
</body>
</html>`))

var browserHomeCardTemplate = template.Must(browserHomeTemplate.New("resource-card").Parse(`<article class="resource-card" data-search="{{.SearchText}}" data-kind="{{.Kind}}">
  <div class="card-body">
    <div class="card-heading">
      <span class="resource-mark" aria-hidden="true">{{.Monogram}}</span>
      <div class="title-group">
        <h3 class="resource-name">{{.Name}}</h3>
        {{if .Kind}}<span class="type-badge kind-{{.Kind}}">{{.KindLabel}}</span>{{end}}
      </div>
    </div>
    {{if .Description}}<p class="description">{{.Description}}</p>{{end}}
  </div>
  <div class="address-list">
    {{if .Primary.URL}}<a class="resource-link" href="{{.Primary.URL}}" target="_blank" rel="noopener noreferrer" aria-label="打开 {{.Name}}：{{.Primary.Host}}"><span><span class="address-host">{{.Primary.Host}}</span><span class="address-meta">{{.Primary.Protocol}} · {{.Primary.Ports}}</span></span><span class="open-action">打开 <span aria-hidden="true">↗</span></span></a>{{else}}<div class="address-static"><span class="address-host">{{.Primary.Host}}</span><span class="address-meta">{{.Primary.Protocol}} · {{.Primary.Ports}}</span></div>{{end}}
    {{if .Additional}}<details class="addresses"><summary><span>其他可用地址</span><span>{{.Additional | len}} 项</span></summary>{{$resource := .}}{{range $address := .Additional}}{{if $address.URL}}<a class="resource-link" href="{{$address.URL}}" target="_blank" rel="noopener noreferrer" aria-label="打开 {{$resource.Name}}：{{$address.Host}}"><span><span class="address-host">{{$address.Host}}</span><span class="address-meta">{{$address.Protocol}} · {{$address.Ports}}</span></span><span class="open-action">打开 <span aria-hidden="true">↗</span></span></a>{{else}}<div class="address-static"><span class="address-host">{{$address.Host}}</span><span class="address-meta">{{$address.Protocol}} · {{$address.Ports}}</span></div>{{end}}{{end}}</details>{{end}}
  </div>
</article>`))

const browserHomeInitialLimit = 18

func buildBrowserHomeResources(sourceResources []client.Resource) []browserHomeResource {
	resources := make([]browserHomeResource, 0, len(sourceResources))
	for _, source := range sourceResources {
		addresses := make([]browserHomeAddress, 0, len(source.Addresses))
		searchParts := []string{source.Name, source.Description}
		seenAddresses := make(map[string]struct{}, len(source.Addresses))
		for _, address := range source.Addresses {
			host := strings.TrimSpace(address.Host)
			if host == "" {
				continue
			}
			protocol := strings.ToLower(strings.TrimSpace(address.Protocol))
			key := fmt.Sprintf("%s\x00%s\x00%d\x00%d", strings.ToLower(host), protocol, address.PortMin, address.PortMax)
			if _, found := seenAddresses[key]; found {
				continue
			}
			seenAddresses[key] = struct{}{}
			ports := portRange(address.PortMin, address.PortMax)
			addresses = append(addresses, browserHomeAddress{
				Host:     host,
				URL:      browserAddressURL(host, address),
				Protocol: strings.ToUpper(protocol),
				Ports:    ports,
			})
			searchParts = append(searchParts, host, protocol, ports)
		}
		if len(addresses) == 0 {
			continue
		}

		primaryIndex := 0
		for index, address := range addresses {
			if address.URL != "" {
				primaryIndex = index
				break
			}
		}
		primary := addresses[primaryIndex]
		additional := make([]browserHomeAddress, 0, len(addresses)-1)
		additional = append(additional, addresses[:primaryIndex]...)
		additional = append(additional, addresses[primaryIndex+1:]...)

		name := strings.TrimSpace(source.Name)
		if name == "" {
			name = primary.Host
		}
		kind, kindLabel := inferResourceType(source)
		searchParts = append(searchParts, kind, kindLabel)
		resources = append(resources, browserHomeResource{
			Name:        name,
			Description: strings.TrimSpace(source.Description),
			SearchText:  strings.ToLower(strings.Join(searchParts, " ")),
			Monogram:    resourceMonogram(name),
			Kind:        kind,
			KindLabel:   kindLabel,
			Primary:     primary,
			Additional:  additional,
		})
	}
	sort.SliceStable(resources, func(i, j int) bool {
		left, right := strings.ToLower(resources[i].Name), strings.ToLower(resources[j].Name)
		if left == right {
			return resources[i].Name < resources[j].Name
		}
		return left < right
	})
	return resources
}

func splitBrowserHomeResources(resources []browserHomeResource) (initial, deferred []browserHomeResource) {
	limit := min(len(resources), browserHomeInitialLimit)
	return resources[:limit], resources[limit:]
}

func isLoopbackAuthority(authority string) bool {
	host, _, err := net.SplitHostPort(authority)
	if err != nil {
		host = authority
	}
	host = strings.TrimSuffix(strings.Trim(strings.TrimSpace(host), "[]"), ".")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func newBrowserHomeHandler(data browserHomeData, deferred []browserHomeResource) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store, max-age=0")
		writer.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; connect-src 'self'; form-action 'none'; frame-ancestors 'none'; img-src data:; style-src 'unsafe-inline'; script-src 'unsafe-inline'")
		writer.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		writer.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		writer.Header().Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(), usb=()")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		if !isLoopbackAuthority(request.Host) {
			http.Error(writer, "forbidden", http.StatusForbidden)
			return
		}
		if request.Method != http.MethodGet {
			writer.Header().Set("Allow", http.MethodGet)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		switch request.URL.Path {
		case "/":
			if err := browserHomeTemplate.Execute(writer, data); err != nil {
				log.Printf("Render managed browser home page failed: %v", err)
			}
		case "/resources":
			for _, resource := range deferred {
				if err := browserHomeCardTemplate.Execute(writer, resource); err != nil {
					log.Printf("Render deferred managed browser resource failed: %v", err)
					return
				}
			}
		default:
			http.NotFound(writer, request)
		}
	})
	return mux
}

func StartBrowserHome(sourceResources []client.Resource, proxyAddress string) (string, error) {
	resources := buildBrowserHomeResources(sourceResources)
	initial, deferred := splitBrowserHomeResources(resources)
	data := browserHomeData{
		Resources:       initial,
		SSHCommand:      sshProxyCommand(proxyAddress),
		SSHCommandShell: sshProxyCommandShell(),
		HasMore:         len(deferred) != 0,
		InitialLimit:    browserHomeInitialLimit,
		Total:           len(resources),
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("start managed browser home page: %w", err)
	}
	handler := newBrowserHomeHandler(data, deferred)
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
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
	host = strings.TrimSpace(host)
	if host == "" || strings.HasPrefix(host, "*.") || strings.ContainsAny(host, "/ \t\r\n") || isIPRange(host) {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		return ""
	}
	protocol := strings.ToLower(strings.TrimSpace(resource.Protocol))
	if protocol != "tcp" && protocol != "all" {
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
	return sshProxyCommandLine(runtime.GOOS, helper, proxyAddress)
}

func sshProxyCommandLine(goos, helper, proxyAddress string) string {
	if goos == "windows" {
		proxyCommand := fmt.Sprintf(`ProxyCommand="%s" --proxy %s --target %%h:%%p`, helper, proxyAddress)
		proxyCommand = strings.ReplaceAll(proxyCommand, `'`, `''`)
		return fmt.Sprintf(`$proxyCommand = '%s'; ssh -o $proxyCommand USER@HOST`, proxyCommand)
	}
	helper = strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(helper)
	proxyCommand := fmt.Sprintf(`ProxyCommand="%s" --proxy %s --target %%h:%%p`, helper, proxyAddress)
	return fmt.Sprintf("ssh -o %s USER@HOST", shellSingleQuote(proxyCommand))
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func sshProxyCommandShell() string {
	if runtime.GOOS == "windows" {
		return "Windows PowerShell"
	}
	return "终端"
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
	if len(parts) >= 2 {
		first, second := []rune(parts[0]), []rune(parts[1])
		if len(first) != 0 && len(second) != 0 {
			return strings.ToUpper(string(first[0]) + string(second[0]))
		}
	}
	runes := []rune(name)
	if len(runes) >= 2 {
		return strings.ToUpper(string(runes[:2]))
	}
	return strings.ToUpper(string(runes))
}

// inferResourceType classifies a resource by the transports and ports exposed
// so the portal can distinguish browser links from client-only resources.
func inferResourceType(source client.Resource) (kind, label string) {
	hasWeb, hasSSH, hasWide, hasTCP, hasUDP := false, false, false, false, false
	for _, address := range source.Addresses {
		protocol := strings.ToLower(strings.TrimSpace(address.Protocol))
		switch protocol {
		case "tcp":
			hasTCP = true
		case "udp":
			hasUDP = true
			continue
		case "all":
			hasTCP, hasUDP = true, true
		default:
			continue
		}
		if address.PortMin <= 22 && address.PortMax >= 22 {
			hasSSH = true
		}
		if (address.PortMin <= 80 && address.PortMax >= 80) || (address.PortMin <= 443 && address.PortMax >= 443) {
			hasWeb = true
		}
		if address.PortMin <= 1 && address.PortMax >= 65535 {
			hasWide = true
		}
	}
	switch {
	case hasWide:
		return "all", "全端口"
	case hasSSH && hasWeb:
		return "mix", "混合"
	case hasWeb:
		return "web", "Web"
	case hasSSH:
		return "ssh", "SSH"
	case hasTCP && hasUDP:
		return "mix", "TCP / UDP"
	case hasUDP:
		return "udp", "UDP"
	default:
		return "tcp", "TCP"
	}
}
