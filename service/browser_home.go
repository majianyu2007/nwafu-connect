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
<link rel="icon" href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 24 24'><rect width='24' height='24' rx='6' fill='%230066cc'/><text x='12' y='17' font-family='-apple-system,Helvetica Neue,sans-serif' font-size='14' font-weight='700' text-anchor='middle' fill='%23ffffff'>N</text></svg>">
<style>
:root {
  color-scheme: light;
  --accent: #0066cc;
  --focus: #0071e3;
  --sky-link: #2997ff;
  --ink: #1d1d1f;
  --ink-secondary: #666666;
  --ink-muted: #7a7a7a;
  --canvas: #ffffff;
  --parchment: #f5f5f7;
  --dark-tile: #272729;
  --nav-black: #000000;
  --line: #e0e0e0;
  --line-subtle: rgba(0, 0, 0, .08);
  --font-display: "SF Pro Display", "SF Pro Text", -apple-system, BlinkMacSystemFont, "PingFang SC", "Helvetica Neue", Helvetica, Arial, sans-serif;
  --font-text: "SF Pro Text", -apple-system, BlinkMacSystemFont, "PingFang SC", "Helvetica Neue", Helvetica, Arial, sans-serif;
  --ease: cubic-bezier(0.25, 0.1, 0.25, 1);
}
* { box-sizing: border-box; }
[hidden] { display: none !important; }
html { -webkit-font-smoothing: antialiased; -moz-osx-font-smoothing: grayscale; }
body {
  margin: 0;
  background: var(--parchment);
  color: var(--ink);
  font-family: var(--font-text);
  font-size: 17px;
  font-weight: 400;
  line-height: 1.47;
  letter-spacing: -0.374px;
}
button, input { color: inherit; font: inherit; }
button, a { touch-action: manipulation; }
a { color: var(--accent); }
.wrap { width: min(1040px, calc(100% - 44px)); margin: 0 auto; }
.skip-link {
  position: fixed; z-index: 1000; top: 8px; left: 8px; padding: 8px 14px;
  border-radius: 9999px; background: var(--canvas); color: var(--accent);
  font-size: 14px; font-weight: 600; text-decoration: none; transform: translateY(-160%);
}
.skip-link:focus { transform: none; outline: 2px solid var(--focus); outline-offset: 2px; }

/* 44px Global Navigation Bar */
.global-nav {
  background: var(--nav-black);
  color: rgba(255, 255, 255, .85);
}
.global-nav .wrap {
  display: flex; min-height: 44px; align-items: center; justify-content: space-between; gap: 16px;
}
.brand {
  display: inline-flex; align-items: center; gap: 8px;
  color: #ffffff; font-size: 13px; font-weight: 600; letter-spacing: -0.08px;
}
.brand-mark {
  display: grid; width: 22px; height: 22px; place-items: center;
  border-radius: 6px; background: var(--accent); color: #ffffff;
  font-family: var(--font-display); font-size: 12px; font-weight: 700; letter-spacing: 0;
}
.connection-state {
  display: inline-flex; align-items: center; gap: 7px;
  color: rgba(255, 255, 255, .8); font-size: 12px; font-weight: 400; letter-spacing: 0;
}
.status-dot { width: 7px; height: 7px; border-radius: 50%; background: #30d158; }

/* 52px Sticky Frosted Sub-Navigation & Discovery Controls */
.sub-nav {
  position: sticky; z-index: 30; top: 0;
  background: rgba(245, 245, 247, .82);
  -webkit-backdrop-filter: saturate(180%) blur(20px);
  backdrop-filter: saturate(180%) blur(20px);
  border-bottom: 1px solid var(--line-subtle);
}
.sub-nav-inner {
  display: flex; min-height: 56px; align-items: center; justify-content: space-between;
  gap: 16px; padding: 8px 0;
}
.sub-nav-title {
  flex: 0 0 auto; color: var(--ink);
  font-family: var(--font-display); font-size: 21px; font-weight: 600;
  line-height: 1.19; letter-spacing: -0.22px;
}
.discovery-tools {
  display: flex; flex: 1 1 auto; align-items: center; justify-content: flex-end; gap: 12px; min-width: 0;
}
.search-control {
  display: flex; flex: 0 1 320px; min-width: 180px; height: 38px; align-items: center; gap: 8px;
  padding: 0 12px; border: 1px solid var(--line); border-radius: 9999px; background: var(--canvas);
  transition: border-color .16s var(--ease);
}
.search-control:focus-within { border-color: var(--focus); outline: 2px solid var(--focus); outline-offset: 1px; }
.search-icon { display: grid; flex: 0 0 auto; color: var(--ink-muted); }
#resourceSearch {
  width: 100%; min-width: 0; border: 0; outline: 0; background: transparent;
  font-size: 14px; font-weight: 400; letter-spacing: -0.224px;
}
#resourceSearch::placeholder { color: var(--ink-muted); }
.shortcut {
  flex: 0 0 auto; padding: 2px 7px; border: 1px solid var(--line);
  border-radius: 9999px; background: var(--parchment); color: var(--ink-muted);
  font-size: 11px; font-weight: 600; letter-spacing: 0; white-space: nowrap;
}
.filters { display: flex; flex: 0 0 auto; gap: 6px; }
.filter-button {
  min-height: 36px; padding: 0 14px; border: 1px solid var(--line); border-radius: 9999px;
  background: var(--canvas); color: var(--ink); cursor: pointer;
  font-size: 13px; font-weight: 400; letter-spacing: -0.1px;
  transition: border-color .16s var(--ease), color .16s var(--ease), transform .12s var(--ease);
}
.filter-button:hover { border-color: var(--ink-muted); }
.filter-button:active { transform: scale(0.95); }
.filter-button[aria-pressed="true"] {
  border: 2px solid var(--focus); padding: 0 13px;
  color: var(--ink); font-weight: 600;
}
.filter-button:focus-visible, .secondary-button:focus-visible, .copy-button:focus-visible, summary:focus-visible {
  outline: 2px solid var(--focus); outline-offset: 2px;
}

/* Pure White Hero Tile */
.hero-tile {
  background: var(--canvas);
  border-bottom: 1px solid var(--line-subtle);
}
.portal-intro {
  display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: end; gap: 36px;
  padding: 56px 0 48px;
}
.session-label {
  margin: 0 0 8px; color: var(--accent);
  font-size: 14px; font-weight: 600; letter-spacing: -0.224px;
}
h1 {
  margin: 0; color: var(--ink);
  font-family: var(--font-display); font-size: clamp(34px, 5vw, 56px);
  font-weight: 600; line-height: 1.07; letter-spacing: -0.28px;
}
.intro-copy {
  max-width: 640px; margin: 14px 0 0; color: var(--ink-secondary);
  font-size: 17px; font-weight: 400; line-height: 1.47; letter-spacing: -0.374px;
}
.resource-total {
  min-width: 136px; padding-left: 28px; border-left: 1px solid var(--line);
}
.resource-total strong {
  display: block; color: var(--ink);
  font-family: var(--font-display); font-size: 40px; font-weight: 600;
  line-height: 1.05; letter-spacing: -0.28px; font-variant-numeric: tabular-nums;
}
.resource-total span {
  display: block; margin-top: 6px; color: var(--ink-secondary);
  font-size: 14px; font-weight: 400; letter-spacing: -0.224px;
}

/* Parchment Resource Grid Tile */
.grid-tile {
  background: var(--parchment);
  padding: 48px 0 64px;
}
.section-heading {
  display: flex; align-items: baseline; justify-content: space-between; gap: 20px; margin-bottom: 24px;
}
.section-heading h2 {
  margin: 0; color: var(--ink);
  font-family: var(--font-display); font-size: clamp(24px, 3vw, 32px);
  font-weight: 600; line-height: 1.14; letter-spacing: -0.28px;
}
.section-heading p {
  margin: 6px 0 0; color: var(--ink-secondary);
  font-size: 14px; font-weight: 400; letter-spacing: -0.224px;
}
#resultCount {
  color: var(--ink-muted); font-size: 14px; font-weight: 400;
  font-variant-numeric: tabular-nums; white-space: nowrap;
}
.resource-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px; }
.resource-card {
  display: flex; min-width: 0; flex-direction: column; overflow: hidden;
  border: 1px solid var(--line); border-radius: 18px; background: var(--canvas);
  transition: border-color .16s var(--ease);
}
.resource-card:hover { border-color: #b8b8be; }
.card-body { display: flex; min-height: 132px; flex: 1; flex-direction: column; padding: 20px 20px 16px; }
.card-heading { display: flex; gap: 12px; align-items: flex-start; }
.resource-mark {
  display: grid; flex: 0 0 auto; width: 42px; height: 42px; place-items: center;
  border-radius: 10px; background: var(--parchment); color: var(--ink);
  font-family: var(--font-display); font-size: 14px; font-weight: 600; letter-spacing: 0;
}
.title-group { min-width: 0; }
.resource-name {
  margin: 0; overflow-wrap: anywhere; color: var(--ink);
  font-family: var(--font-display); font-size: 17px; font-weight: 600;
  line-height: 1.29; letter-spacing: -0.22px;
}
.type-badge {
  display: inline-flex; margin-top: 6px; padding: 2px 9px; border-radius: 9999px;
  background: var(--parchment); color: var(--ink-secondary);
  font-size: 12px; font-weight: 400; letter-spacing: 0;
}
.type-badge.kind-web, .type-badge.kind-all, .type-badge.kind-mix {
  background: rgba(0, 102, 204, .09); color: var(--accent); font-weight: 600;
}
.description {
  display: -webkit-box; overflow: hidden; margin: 12px 0 0; color: var(--ink-secondary);
  font-size: 14px; font-weight: 400; line-height: 1.43; letter-spacing: -0.224px;
  -webkit-box-orient: vertical; -webkit-line-clamp: 2;
}
.address-list { border-top: 1px solid var(--line-subtle); background: #fafafc; }
.resource-link, .address-static {
  display: grid; min-height: 48px; grid-template-columns: minmax(0, 1fr) auto;
  gap: 12px; align-items: center; padding: 10px 20px; text-decoration: none;
}
.resource-link > span:first-child { display: flex; min-width: 0; align-items: baseline; gap: 6px; }
.resource-link { color: var(--accent); transition: background-color .14s var(--ease); }
.resource-link:hover { background: var(--parchment); }
.resource-link:focus-visible { outline: 2px solid var(--focus); outline-offset: -2px; }
.address-host {
  min-width: 0; overflow: hidden; font-size: 13px; font-weight: 600;
  text-overflow: ellipsis; white-space: nowrap; letter-spacing: 0;
}
.address-meta {
  color: var(--ink-muted); font-size: 12px; font-weight: 400;
  text-transform: uppercase; white-space: nowrap; letter-spacing: 0;
}
.open-action {
  color: var(--accent); font-size: 13px; font-weight: 400; white-space: nowrap; letter-spacing: 0;
}
.resource-link:hover .open-action { text-decoration: underline; }
.address-static .address-host { color: var(--ink-secondary); font-weight: 400; }
details.addresses { border-top: 1px solid var(--line-subtle); }
details.addresses summary {
  display: flex; min-height: 44px; align-items: center; justify-content: space-between; gap: 12px;
  padding: 8px 20px; color: var(--ink-secondary); cursor: pointer;
  font-size: 13px; font-weight: 400; list-style: none;
}
details.addresses summary::-webkit-details-marker { display: none; }
details.addresses summary::after { content: "展开"; color: var(--accent); font-weight: 400; }
details.addresses[open] summary::after { content: "收起"; }
details.addresses .resource-link, details.addresses .address-static {
  border-top: 1px solid var(--line-subtle); padding-left: 24px;
}

.empty-state {
  padding: 64px 24px; border: 1px solid var(--line); border-radius: 18px;
  background: var(--canvas); color: var(--ink-secondary); text-align: center;
}
.empty-state strong {
  display: block; margin-bottom: 6px; color: var(--ink);
  font-family: var(--font-display); font-size: 21px; font-weight: 600; letter-spacing: -0.22px;
}
.empty-state span { font-size: 14px; }
.secondary-button {
  min-height: 44px; padding: 0 22px; border: 1px solid var(--accent); border-radius: 9999px;
  background: transparent; color: var(--accent); cursor: pointer;
  font-size: 15px; font-weight: 400; letter-spacing: -0.224px;
  transition: background-color .16s var(--ease), color .16s var(--ease), transform .12s var(--ease);
}
.secondary-button:hover { background: var(--accent); color: #ffffff; }
.secondary-button:active { transform: scale(0.95); }
.empty-state .secondary-button { margin-top: 20px; }
.list-actions { display: flex; justify-content: center; margin-top: 28px; }

/* Full-Bleed Dark Showcase Tile for SSH/Developer Access & Policy */
.dark-tile {
  background: var(--dark-tile);
  color: #ffffff;
  border-radius: 0;
  padding: 64px 0;
}
.dark-grid {
  display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1.35fr); gap: 40px; align-items: start;
}
.dark-eyebrow {
  margin: 0 0 8px; color: var(--sky-link);
  font-size: 13px; font-weight: 600; letter-spacing: 0;
}
.dark-tile h2 {
  margin: 0; color: #ffffff;
  font-family: var(--font-display); font-size: 28px; font-weight: 600;
  line-height: 1.14; letter-spacing: -0.28px;
}
.dark-copy {
  margin: 12px 0 0; color: #cccccc;
  font-size: 15px; font-weight: 400; line-height: 1.5; letter-spacing: -0.224px;
}
.client-access {
  border: 1px solid rgba(255, 255, 255, .14); border-radius: 18px;
  background: rgba(255, 255, 255, .04);
}
.client-access summary {
  display: grid; min-height: 62px; grid-template-columns: minmax(0, 1fr) auto; align-items: center;
  column-gap: 20px; row-gap: 3px; padding: 14px 20px; cursor: pointer; list-style: none;
}
.client-access summary::-webkit-details-marker { display: none; }
.client-access summary strong {
  grid-column: 1; grid-row: 1; color: #ffffff;
  font-family: var(--font-display); font-size: 17px; font-weight: 600; letter-spacing: -0.22px;
}
.client-access summary > span {
  grid-column: 1; grid-row: 2; color: #a6a6a6; font-size: 13px; font-weight: 400;
}
.client-access summary::after {
  content: "展开"; grid-column: 2; grid-row: 1 / span 2;
  color: var(--sky-link); font-size: 14px; font-weight: 400; white-space: nowrap;
}
.client-access[open] summary::after { content: "收起"; }
.client-guide { padding: 0 20px 20px; border-top: 1px solid rgba(255, 255, 255, .1); }
.client-guide p { margin: 16px 0 0; color: #cccccc; font-size: 14px; line-height: 1.5; }
.client-guide code {
  display: block; overflow-x: auto; margin-top: 12px; padding: 14px 16px;
  border-radius: 12px; background: #000000; color: #f5f5f7;
  font: 12.5px/1.55 "SF Mono", Menlo, Consolas, monospace; white-space: nowrap;
}
.copy-button {
  margin-top: 14px; min-height: 40px; padding: 0 20px; border: 0; border-radius: 9999px;
  background: var(--accent); color: #ffffff; cursor: pointer;
  font-size: 14px; font-weight: 400; letter-spacing: -0.224px;
  transition: background-color .16s var(--ease), transform .12s var(--ease);
}
.copy-button:hover { background: var(--focus); }
.copy-button:active { transform: scale(0.95); }
.copy-button.copied { background: #1d7a46; }

footer {
  background: var(--parchment);
  padding: 24px 0 32px;
  color: var(--ink-muted);
  font-size: 12px;
  font-weight: 400;
  letter-spacing: 0;
  text-align: center;
}

@media (max-width: 960px) {
  .resource-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .sub-nav-inner { flex-wrap: wrap; padding: 10px 0; }
  .discovery-tools { width: 100%; justify-content: space-between; }
  .search-control { flex: 1 1 240px; }
  .dark-grid { grid-template-columns: 1fr; gap: 28px; }
}
@media (max-width: 640px) {
  .wrap { width: min(100% - 32px, 1040px); }
  .connection-state span:last-child { font-size: 0; }
  .connection-state span:last-child::after { content: "已连接"; font-size: 12px; }
  .sub-nav { position: static; }
  .discovery-tools { flex-direction: column; align-items: stretch; }
  .filters { overflow-x: auto; padding-bottom: 2px; }
  .portal-intro { grid-template-columns: 1fr; gap: 20px; padding: 36px 0 32px; }
  .resource-total { display: flex; min-width: 0; align-items: baseline; gap: 8px; padding: 0; border: 0; }
  .resource-total strong { font-size: 28px; }
  .resource-total span { margin: 0; }
  .grid-tile { padding: 36px 0 48px; }
  .section-heading { align-items: flex-start; flex-direction: column; gap: 6px; }
  .resource-grid { grid-template-columns: 1fr; }
  .card-body { min-height: 0; }
  .shortcut { display: none; }
  .dark-tile { padding: 48px 0; }
}
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { scroll-behavior: auto !important; transition-duration: .01ms !important; }
}
</style>
</head>
<body>
<a class="skip-link" href="#resourceSearch">跳到资源搜索</a>
<header class="global-nav">
  <div class="wrap">
    <div class="brand"><span class="brand-mark" aria-hidden="true">N</span><span>NWAFU Connect</span></div>
    <div class="connection-state"><span class="status-dot" aria-hidden="true"></span><span>aTrust 安全连接已建立</span></div>
  </div>
</header>
<nav class="sub-nav" aria-label="资源导航与查找">
  <div class="wrap sub-nav-inner">
    <span class="sub-nav-title">校内资源</span>
    <div class="discovery-tools">
      <div class="search-control">
        <span class="search-icon" aria-hidden="true"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="11" cy="11" r="7"/><path d="m20 20-4-4"/></svg></span>
        <input id="resourceSearch" type="search" aria-label="查找资源" placeholder="搜索名称、说明或地址" autocomplete="off" spellcheck="false">
        <span class="shortcut" id="searchShortcut">⌘ K</span>
      </div>
      <div class="filters" role="group" aria-label="按资源类型筛选">
        <button class="filter-button" type="button" data-kind="all" aria-pressed="true">全部</button>
        <button class="filter-button" type="button" data-kind="web" aria-pressed="false">网页</button>
        <button class="filter-button" type="button" data-kind="ssh" aria-pressed="false">SSH</button>
        <button class="filter-button" type="button" data-kind="client" aria-pressed="false">客户端</button>
      </div>
    </div>
  </div>
</nav>
<main>
  <section class="hero-tile" aria-labelledby="pageTitle">
    <div class="wrap portal-intro">
      <div>
        <p class="session-label">本次会话已授权</p>
        <h1 id="pageTitle">校内资源</h1>
        <p class="intro-copy">这里仅展示学校 aTrust 网关为当前账号下发的访问权限。浏览校内服务期间，请保持 NWAFU Connect 运行。</p>
      </div>
      <div class="resource-total" aria-label="共 {{.Total}} 项资源"><strong>{{.Total}}</strong><span>项可用资源</span></div>
    </div>
  </section>

  <section class="grid-tile" aria-labelledby="resourceHeading">
    <div class="wrap">
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
    </div>
  </section>

  <section class="dark-tile" aria-label="权限说明与开发者客户端接入">
    <div class="wrap dark-grid">
      <aside>
        <p class="dark-eyebrow">访问边界</p>
        <h2>列表中没有需要的网站？</h2>
        <p class="dark-copy">受管浏览器严格遵守网关路由策略，不会绕过学校权限或回退到本机直连。如需新增目标，请联系学校网络管理员将其加入你的 aTrust 资源策略。</p>
      </aside>
      <details class="client-access">
        <summary><strong>SSH / SFTP 等 TCP 客户端接入</strong><span>高级用法 · 普通网页访问无需设置</span></summary>
        <div class="client-guide">
          <p>使用随应用附带的 stdio 代理助手，可将 SSH、SFTP、Git 等 TCP 客户端接入当前会话；请复制到 {{.SSHCommandShell}} 运行：</p>
          <code id="sshCommand">{{.SSHCommand}}</code>
          <button class="copy-button" type="button" data-copy="sshCommand">复制命令</button>
        </div>
      </details>
    </div>
  </section>
</main>
<footer>
  <div class="wrap">NWAFU Connect 受管浏览器会话 · 仅代理校园网关授权资源 · 关闭连接后资源将不可访问</div>
</footer>
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
