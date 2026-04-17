package ppfallback

import (
	"fmt"
	"html"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type FallbackHandler struct {
	siteType   string
	proxy      *httputil.ReverseProxy
	inviteCode string
	db         *FallbackDB
}

func NewFallbackHandler(fallbackType, proxyAddress, inviteCode string, db *FallbackDB) (*FallbackHandler, error) {
	if fallbackType == "" || fallbackType == "proxy" {
		fallbackURL, err := url.Parse("http://" + proxyAddress)
		if err != nil {
			return nil, err
		}
		return &FallbackHandler{
			siteType:   "proxy",
			proxy:      httputil.NewSingleHostReverseProxy(fallbackURL),
			inviteCode: inviteCode,
			db:         db,
		}, nil
	}

	siteType := fallbackType
	if siteType != "forum" {
		siteType = "blog"
	}

	return &FallbackHandler{
		siteType:   siteType,
		inviteCode: inviteCode,
		db:         db,
	}, nil
}

func (h *FallbackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.siteType == "proxy" {
		if h.proxy != nil {
			h.proxy.ServeHTTP(w, r)
		} else {
			http.NotFound(w, r)
		}
		return
	}

	switch {
	case r.URL.Path == "/":
		h.serveIndex(w, r)
	case r.URL.Path == "/login":
		h.serveLogin(w, r)
	case r.URL.Path == "/register":
		h.serveRegister(w, r)
	case strings.HasPrefix(r.URL.Path, "/article/"):
		h.serveArticleRoute(w, r, "/article/")
	case strings.HasPrefix(r.URL.Path, "/thread/"):
		h.serveArticleRoute(w, r, "/thread/")
	default:
		http.NotFound(w, r)
	}
}

func (h *FallbackHandler) serveArticleRoute(w http.ResponseWriter, r *http.Request, prefix string) {
	id, action, err := parseArticlePath(r.URL.Path, prefix)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	switch action {
	case "":
		h.serveArticle(w, r, id, prefix)
	case "comment", "comments":
		h.serveCommentGate(w, r, id, prefix)
	case "comments/submit":
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		h.serveCommentSubmit(w, r, id, prefix)
	default:
		http.NotFound(w, r)
	}
}

func parseArticlePath(path string, prefix string) (int, string, error) {
	rest := strings.TrimPrefix(path, prefix)
	if rest == path || rest == "" {
		return 0, "", fmt.Errorf("missing article id")
	}

	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || len(parts) > 3 {
		return 0, "", fmt.Errorf("unsupported route")
	}

	id, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", err
	}

	if len(parts) == 1 {
		return id, "", nil
	}

	return id, strings.Join(parts[1:], "/"), nil
}

func (h *FallbackHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	articles, err := h.db.GetRecentArticles(12)
	if err != nil {
		http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	if h.siteType == "forum" {
		fmt.Fprint(w, forumIndexHTML(articles))
		return
	}
	fmt.Fprint(w, blogIndexHTML(articles))
}

func (h *FallbackHandler) serveArticle(w http.ResponseWriter, r *http.Request, id int, prefix string) {
	article, err := h.db.GetArticle(id)
	if err != nil {
		http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}
	if article == nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	if h.siteType == "forum" {
		fmt.Fprint(w, forumThreadHTML(article, commentPath(prefix, id)))
		return
	}
	fmt.Fprint(w, blogArticleHTML(article, commentPath(prefix, id)))
}

func (h *FallbackHandler) serveCommentGate(w http.ResponseWriter, r *http.Request, id int, prefix string) {
	article, err := h.db.GetArticle(id)
	if err != nil {
		http.Error(w, "Внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}
	if article == nil {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, commentGateHTML(article, articlePath(prefix, id)))
}

func (h *FallbackHandler) serveRegister(w http.ResponseWriter, r *http.Request) {
	errMessage := ""
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		invite := strings.TrimSpace(r.FormValue("invite_code"))
		if invite == "" {
			errMessage = "Для запроса доступа нужен код приглашения."
		} else if h.inviteCode != "" && invite != h.inviteCode {
			errMessage = "Код приглашения не найден. Проверьте написание или запросите новый."
		} else {
			errMessage = "Подтверждение заявок временно недоступно. Попробуйте позже."
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, authPageHTML("Запрос доступа", "Новые профили активируются по персональному коду.", errMessage, true))
}

func (h *FallbackHandler) serveLogin(w http.ResponseWriter, r *http.Request) {
	errMessage := ""
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		invite := strings.TrimSpace(r.FormValue("invite_code"))
		if invite == "" {
			errMessage = "Чтобы войти, введите код доступа из приглашения."
		} else if h.inviteCode != "" && invite != h.inviteCode {
			errMessage = "Код доступа не подошёл. Проверьте написание или запросите новый."
		} else {
			errMessage = "Сервис входа временно недоступен. Повторите попытку позже."
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, authPageHTML("Вход", "Обсуждения, ответы и сохранённые материалы доступны участникам клуба.", errMessage, false))
}

func (h *FallbackHandler) serveCommentSubmit(w http.ResponseWriter, r *http.Request, id int, prefix string) {
	http.Redirect(w, r, commentPath(prefix, id), http.StatusSeeOther)
}

func articlePath(prefix string, id int) string {
	return fmt.Sprintf("%s%d", prefix, id)
}

func commentPath(prefix string, id int) string {
	return fmt.Sprintf("%s%d/comment", prefix, id)
}

func blogIndexHTML(articles []Article) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="ru">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Тихая Сеть</title>
	<style>
		:root { color-scheme: light; --bg:#f4efe7; --panel:rgba(255,251,245,.88); --panel-strong:#fffdf9; --ink:#1f2428; --muted:#666d6c; --line:rgba(103,78,54,.16); --accent:#96482a; --accent-strong:#78331b; --accent-soft:#f1dfd3; --shadow:0 24px 60px rgba(62,38,21,.08); }
		* { box-sizing:border-box; }
		body { margin:0; font-family:"Iowan Old Style","Palatino Linotype","Book Antiqua",Georgia,serif; color:var(--ink); background:radial-gradient(circle at top left, rgba(191,133,90,.18), transparent 28%), radial-gradient(circle at 88% 8%, rgba(157,114,73,.12), transparent 24%), linear-gradient(180deg,#eee3d4 0,#f7f3ec 32%,#f4efe7 100%); }
		a { color:inherit; text-decoration:none; }
		.shell { max-width:1180px; margin:0 auto; padding:24px 20px 72px; }
		.topbar { display:flex; justify-content:space-between; align-items:flex-start; gap:18px; }
		.brand .eyebrow { display:inline-block; margin-bottom:14px; font-size:12px; letter-spacing:.18em; text-transform:uppercase; color:#896f58; }
		.brand h1 { margin:0; font-size:54px; line-height:1; letter-spacing:-.04em; }
		.brand p { margin:12px 0 0; max-width:560px; color:var(--muted); font-size:18px; line-height:1.6; }
		.actions { display:flex; gap:12px; flex-wrap:wrap; }
		.button { display:inline-flex; align-items:center; justify-content:center; min-height:44px; padding:0 18px; border-radius:999px; font-size:15px; font-weight:700; transition:.2s ease; }
		.button.solid { background:var(--accent); color:#fff8f3; box-shadow:0 14px 30px rgba(120,51,27,.18); }
		.button.ghost { border:1px solid var(--line); background:rgba(255,251,245,.72); color:var(--ink); }
		.hero { display:grid; grid-template-columns:minmax(0,1.35fr) 320px; gap:18px; margin:26px 0 24px; }
		.hero-main, .side-card, .story-card { background:var(--panel); border:1px solid var(--line); border-radius:28px; box-shadow:var(--shadow); }
		.hero-main { position:relative; overflow:hidden; padding:34px; }
		.hero-main::after { content:""; position:absolute; inset:auto -110px -110px auto; width:280px; height:280px; border-radius:50%; background:radial-gradient(circle, rgba(150,72,42,.14), transparent 70%); }
		.hero-kicker, .story-kicker { display:flex; gap:12px; flex-wrap:wrap; align-items:center; font-size:14px; color:var(--muted); }
		.tag { display:inline-flex; align-items:center; min-height:28px; padding:0 12px; border-radius:999px; background:var(--accent-soft); color:var(--accent-strong); font-size:12px; font-weight:700; letter-spacing:.06em; text-transform:uppercase; }
		.hero h2 { margin:18px 0 14px; font-size:40px; line-height:1.05; letter-spacing:-.04em; max-width:720px; }
		.hero p { margin:0; max-width:680px; color:#353a3d; font-size:18px; line-height:1.8; }
		.hero-actions { display:flex; gap:12px; flex-wrap:wrap; margin-top:24px; }
		.sidebar-stack, .sidebar { display:grid; gap:18px; align-content:start; }
		.side-card { padding:24px; }
		.side-card h3 { margin:0 0 12px; font-size:21px; }
		.side-card p { margin:0; color:var(--muted); line-height:1.7; }
		.info-line { display:flex; gap:10px; flex-wrap:wrap; margin-top:16px; color:var(--muted); font-size:14px; }
		.chips { display:flex; gap:10px; flex-wrap:wrap; }
		.chips span { display:inline-flex; align-items:center; min-height:32px; padding:0 12px; border:1px solid var(--line); border-radius:999px; color:#574a41; background:rgba(255,255,255,.6); font-size:14px; }
		.layout { display:grid; grid-template-columns:minmax(0,1fr) 320px; gap:18px; }
		.feed { display:grid; gap:18px; }
		.story-card { padding:24px 26px; }
		.story-card h2 { margin:14px 0 12px; font-size:31px; line-height:1.12; letter-spacing:-.03em; }
		.story-card p { margin:0; color:#3d4347; line-height:1.8; }
		.story-bottom { display:flex; justify-content:space-between; align-items:center; gap:14px; margin-top:18px; color:var(--muted); font-size:14px; }
		.story-bottom a { color:var(--accent); font-weight:700; }
		.story-empty { display:grid; place-items:center; min-height:220px; text-align:center; }
		.story-empty h2 { margin:0 0 12px; }
		.mini-list { list-style:none; padding:0; margin:0; display:grid; gap:12px; }
		.mini-list li { padding-bottom:12px; border-bottom:1px solid rgba(103,78,54,.10); }
		.mini-list li:last-child { padding-bottom:0; border-bottom:none; }
		.mini-list a { font-weight:700; line-height:1.45; }
		.mini-list span { display:block; margin-top:4px; color:var(--muted); font-size:14px; }
		.text-link { color:var(--accent); font-weight:700; }
		@media (max-width: 980px) {
			.hero, .layout { grid-template-columns:1fr; }
			.topbar { flex-direction:column; }
			.hero h2 { font-size:34px; }
		}
		@media (max-width: 640px) {
			.shell { padding:20px 16px 54px; }
			.brand h1 { font-size:42px; }
			.hero-main, .side-card, .story-card { border-radius:24px; }
			.hero-main { padding:26px; }
			.story-card { padding:22px; }
			.story-card h2 { font-size:26px; }
		}
	</style>
</head>
<body>
	<div class="shell">
		<header class="topbar">
			<div class="brand">
				<span class="eyebrow">Русский журнал</span>
				<h1><a href="/">Тихая Сеть</a></h1>
				<p>Наблюдения о сетях, инфраструктуре и самостоятельных сервисах без рекламного шума.</p>
			</div>
			<div class="actions">
				<a class="button ghost" href="/login">Войти</a>
				<a class="button solid" href="/register">Запросить доступ</a>
			</div>
		</header>

		<section class="hero">`)

	if len(articles) == 0 {
		b.WriteString(`<article class="hero-main">
				<div class="hero-kicker">
					<span class="tag">Свежий выпуск</span>
					<span>Редакция в работе</span>
				</div>
				<h2>Новый номер уже собирается</h2>
				<p>Здесь появятся длинные и аккуратно оформленные материалы о практической инфраструктуре, сетях и самостоятельном хостинге.</p>
				<div class="hero-actions">
					<a class="button solid" href="/register">Подписаться на клуб</a>
				</div>
			</article>
			<div class="sidebar-stack">
				<section class="side-card">
					<h3>Разделы</h3>
					<div class="chips">
						<span>Сети</span>
						<span>Инфраструктура</span>
						<span>Практика</span>
					</div>
				</section>
			</div>`)
	} else {
		featured := articles[0]
		fmt.Fprintf(&b, `<article class="hero-main">
				<div class="hero-kicker">
					<span class="tag">%s</span>
					<span>%s</span>
					<span>%s</span>
				</div>
				<h2><a href="/article/%d">%s</a></h2>
				<p>%s</p>
				<div class="hero-actions">
					<a class="button solid" href="/article/%d">Читать материал</a>
					<a class="button ghost" href="/login">Обсудить</a>
				</div>
			</article>
			<div class="sidebar-stack">
				<section class="side-card">
					<h3>Сейчас на обложке</h3>
					<p>%s</p>
					<div class="info-line">
						<span>%s</span>
						<span>%s</span>
					</div>
				</section>
				<section class="side-card">
					<h3>Разделы</h3>
					<div class="chips">%s</div>
				</section>
			</div>`,
			html.EscapeString(articleCategory(featured)),
			html.EscapeString(formatDate(featured.CreatedAt)),
			html.EscapeString(articleReadingTimeLabel(featured.Content)),
			featured.ID,
			html.EscapeString(featured.Title),
			safeSnippet(featured.Content, 360),
			featured.ID,
			html.EscapeString(articleSourceLabel(featured.Link)),
			html.EscapeString(formatDateTime(featured.CreatedAt)),
			html.EscapeString(articleReadingTimeLabel(featured.Content)),
			blogSectionChipsHTML(articles, 5))
	}

	b.WriteString(`</section>

		<main class="layout">
			<section class="feed">`)

	if len(articles) <= 1 {
		b.WriteString(`<article class="story-card story-empty">
				<div>
					<h2>Архив скоро пополнится</h2>
					<p>На главной останутся только аккуратные материалы и спокойные разборы без визуального шума.</p>
				</div>
			</article>`)
	} else {
		for _, a := range articles[1:] {
			fmt.Fprintf(&b, `<article class="story-card">
					<div class="story-kicker">
						<span class="tag">%s</span>
						<span>%s</span>
						<span>%s</span>
					</div>
					<h2><a href="/article/%d">%s</a></h2>
					<p>%s</p>
					<div class="story-bottom">
						<span>%s</span>
						<a href="/article/%d">Открыть</a>
					</div>
				</article>`,
				html.EscapeString(articleCategory(a)),
				html.EscapeString(formatDate(a.CreatedAt)),
				html.EscapeString(articleReadingTimeLabel(a.Content)),
				a.ID,
				html.EscapeString(a.Title),
				safeSnippet(a.Content, 220),
				html.EscapeString(articleSourceLabel(a.Link)),
				a.ID)
		}
	}

	b.WriteString(`</section>
			<aside class="sidebar">`)
	b.WriteString(blogSidebarHTML(articles))
	b.WriteString(`</aside>
		</main>
	</div>
</body>
</html>`)
	return b.String()
}

func forumIndexHTML(articles []Article) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="ru">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Наблюдатель | Форум</title>
	<style>
		:root { color-scheme: light; --bg:#edf2f7; --panel:rgba(255,255,255,.9); --ink:#152233; --muted:#677688; --line:rgba(21,34,51,.10); --accent:#125db1; --accent-dark:#0b478d; --accent-soft:#dfeefe; --shadow:0 22px 54px rgba(16,43,76,.08); }
		* { box-sizing:border-box; }
		body { margin:0; font-family:"IBM Plex Sans","Segoe UI",Tahoma,sans-serif; color:var(--ink); background:radial-gradient(circle at top right, rgba(18,93,177,.10), transparent 25%), radial-gradient(circle at top left, rgba(84,125,170,.10), transparent 20%), linear-gradient(180deg,#dfe8f3 0,#edf2f7 28%,#edf2f7 100%); }
		a { color:inherit; text-decoration:none; }
		.shell { max-width:1240px; margin:0 auto; padding:24px 20px 72px; }
		.topbar { display:flex; justify-content:space-between; align-items:flex-start; gap:18px; }
		.brand small { display:inline-block; margin-bottom:12px; font-size:12px; letter-spacing:.18em; text-transform:uppercase; color:#6d7f95; }
		.brand h1 { margin:0; font-size:46px; line-height:1; letter-spacing:-.04em; }
		.brand p { margin:10px 0 0; max-width:620px; color:var(--muted); line-height:1.65; font-size:17px; }
		.actions { display:flex; gap:12px; flex-wrap:wrap; }
		.button { display:inline-flex; align-items:center; justify-content:center; min-height:44px; padding:0 18px; border-radius:999px; font-size:15px; font-weight:700; transition:.2s ease; }
		.button.solid { background:var(--accent); color:#fff; box-shadow:0 14px 30px rgba(18,93,177,.18); }
		.button.ghost { border:1px solid var(--line); background:rgba(255,255,255,.72); color:var(--ink); }
		.stats { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:14px; margin:24px 0 18px; }
		.stat, .board, .side-card, .post-card { background:var(--panel); border:1px solid var(--line); border-radius:26px; box-shadow:var(--shadow); }
		.stat { padding:18px 20px; }
		.stat strong { display:block; font-size:34px; line-height:1; letter-spacing:-.04em; }
		.stat span { display:block; margin-top:6px; color:var(--muted); font-size:14px; }
		.layout { display:grid; grid-template-columns:minmax(0,1fr) 310px; gap:18px; }
		.board { overflow:hidden; }
		.board-head, .board-row { display:grid; grid-template-columns:minmax(0,1.8fr) 94px 110px 170px; gap:16px; align-items:center; padding:18px 22px; }
		.board-head { background:#f8fbff; color:#5f7389; font-size:13px; font-weight:700; letter-spacing:.08em; text-transform:uppercase; }
		.board-row { border-top:1px solid var(--line); }
		.topic-cell { display:grid; grid-template-columns:54px minmax(0,1fr); gap:14px; align-items:start; }
		.avatar { width:54px; height:54px; border-radius:16px; display:grid; place-items:center; background:linear-gradient(135deg,#1a67bd,#0b478d); color:#fff; font-size:20px; font-weight:700; }
		.topic-top { display:flex; gap:10px; flex-wrap:wrap; align-items:center; margin-bottom:8px; }
		.badge { display:inline-flex; align-items:center; min-height:26px; padding:0 10px; border-radius:999px; background:var(--accent-soft); color:var(--accent-dark); font-size:12px; font-weight:700; letter-spacing:.06em; text-transform:uppercase; }
		.topic-title { display:block; font-size:21px; font-weight:700; line-height:1.25; letter-spacing:-.02em; }
		.topic-copy { margin:8px 0 0; color:var(--muted); line-height:1.65; }
		.topic-meta { margin-top:10px; color:var(--muted); font-size:14px; }
		.metric, .last { color:#223449; font-weight:700; }
		.last { color:var(--muted); font-size:14px; font-weight:600; line-height:1.5; }
		.sidebar { display:grid; gap:18px; align-content:start; }
		.side-card { padding:24px; }
		.side-card h3 { margin:0 0 12px; font-size:21px; }
		.side-card p { margin:0; color:var(--muted); line-height:1.7; }
		.chips { display:flex; flex-wrap:wrap; gap:10px; }
		.chips span { display:inline-flex; align-items:center; min-height:32px; padding:0 12px; border-radius:999px; border:1px solid var(--line); background:#fff; color:#415365; font-size:14px; }
		.mini-list { list-style:none; margin:0; padding:0; display:grid; gap:12px; }
		.mini-list li { padding-bottom:12px; border-bottom:1px solid var(--line); }
		.mini-list li:last-child { padding-bottom:0; border-bottom:none; }
		.mini-list a { font-weight:700; line-height:1.45; }
		.mini-list span { display:block; margin-top:4px; color:var(--muted); font-size:14px; }
		.empty { padding:34px 24px; color:var(--muted); line-height:1.7; }
		@media (max-width: 1080px) {
			.layout { grid-template-columns:1fr; }
		}
		@media (max-width: 920px) {
			.topbar { flex-direction:column; }
			.stats { grid-template-columns:1fr; }
			.board-head { display:none; }
			.board-row { grid-template-columns:1fr; gap:14px; }
			.metric::before { content:attr(data-label) ": "; color:var(--muted); font-weight:600; }
			.last::before { content:"Обновлено: "; color:var(--muted); font-weight:600; }
		}
		@media (max-width: 640px) {
			.shell { padding:20px 16px 54px; }
			.brand h1 { font-size:38px; }
			.stat, .board, .side-card, .post-card { border-radius:22px; }
			.board-row { padding:18px; }
			.topic-cell { grid-template-columns:48px minmax(0,1fr); }
			.avatar { width:48px; height:48px; border-radius:14px; }
			.topic-title { font-size:19px; }
		}
	</style>
</head>
<body>
	<div class="shell">
		<header class="topbar">
			<div class="brand">
				<small>Сообщество</small>
				<h1><a href="/">Наблюдатель</a></h1>
				<p>Разговоры о сетях, безопасности, самохостинге и спокойной инженерной работе.</p>
			</div>
			<div class="actions">
				<a class="button ghost" href="/login">Войти</a>
				<a class="button solid" href="/register">Запросить доступ</a>
			</div>
		</header>

		<section class="stats">`)
	fmt.Fprintf(&b, `<div class="stat">
				<strong>%d</strong>
				<span>тем в фокусе</span>
			</div>
			<div class="stat">
				<strong>%d</strong>
				<span>ответов в архиве</span>
			</div>
			<div class="stat">
				<strong>%d</strong>
				<span>участников клуба</span>
			</div>`,
		len(articles),
		forumTotalReplies(articles),
		forumMemberCount(articles))
	b.WriteString(`
		</section>

		<div class="layout">
			<section class="board">
				<div class="board-head">
					<div>Тема</div>
					<div>Ответы</div>
					<div>Просмотры</div>
					<div>Последнее</div>
				</div>`)

	if len(articles) == 0 {
		b.WriteString(`<div class="empty">Первое обсуждение уже готовится. В ленте останутся только содержательные ветки без лишнего шума.</div>`)
	} else {
		for _, a := range articles {
			fmt.Fprintf(&b, `<div class="board-row">
					<div class="topic-cell">
						<div class="avatar">%s</div>
						<div>
							<div class="topic-top">
								<span class="badge">%s</span>
								<span>%s</span>
							</div>
							<a class="topic-title" href="/thread/%d">%s</a>
							<p class="topic-copy">%s</p>
							<div class="topic-meta">%s · %s</div>
						</div>
					</div>
					<div class="metric" data-label="Ответы">%d</div>
					<div class="metric" data-label="Просмотры">%d</div>
					<div class="last">%s</div>
				</div>`,
				html.EscapeString(articleInitial(a)),
				html.EscapeString(articleCategory(a)),
				html.EscapeString(articleSourceLabel(a.Link)),
				a.ID,
				html.EscapeString(a.Title),
				safeSnippet(a.Content, 150),
				html.EscapeString(forumAuthorName(a.ID)),
				html.EscapeString(formatDate(a.CreatedAt)),
				forumReplyCount(a),
				forumViewCount(a),
				html.EscapeString(formatDateTime(forumLastActivity(a))))
		}
	}

	b.WriteString(`</section>
			<aside class="sidebar">`)
	b.WriteString(forumSidebarHTML(articles))
	b.WriteString(`</aside>
		</div>
	</div>
</body>
</html>`)
	return b.String()
}

func blogArticleHTML(article *Article, commentURL string) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="ru">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>` + html.EscapeString(article.Title) + `</title>
	<style>
		:root { color-scheme: light; --bg:#f4efe7; --panel:rgba(255,251,245,.92); --ink:#1f2428; --muted:#666d6c; --line:rgba(103,78,54,.16); --accent:#96482a; --accent-strong:#78331b; --accent-soft:#f1dfd3; --shadow:0 24px 60px rgba(62,38,21,.08); }
		* { box-sizing:border-box; }
		body { margin:0; font-family:"Iowan Old Style","Palatino Linotype","Book Antiqua",Georgia,serif; color:var(--ink); background:radial-gradient(circle at top left, rgba(191,133,90,.18), transparent 28%), linear-gradient(180deg,#eee3d4 0,#f7f3ec 32%,#f4efe7 100%); }
		a { color:inherit; text-decoration:none; }
		.shell { max-width:1180px; margin:0 auto; padding:24px 20px 72px; }
		.topbar { display:flex; justify-content:space-between; align-items:flex-start; gap:18px; margin-bottom:24px; }
		.back { color:#7b634d; font-size:14px; letter-spacing:.04em; text-transform:uppercase; }
		.actions { display:flex; gap:12px; flex-wrap:wrap; }
		.button { display:inline-flex; align-items:center; justify-content:center; min-height:44px; padding:0 18px; border-radius:999px; font-size:15px; font-weight:700; }
		.button.solid { background:var(--accent); color:#fff8f3; box-shadow:0 14px 30px rgba(120,51,27,.18); }
		.button.ghost { border:1px solid var(--line); background:rgba(255,251,245,.72); }
		.header, .article-card, .side-card { background:var(--panel); border:1px solid var(--line); border-radius:28px; box-shadow:var(--shadow); }
		.header { padding:32px; }
		.kicker { display:flex; gap:12px; flex-wrap:wrap; align-items:center; color:var(--muted); font-size:14px; }
		.tag { display:inline-flex; align-items:center; min-height:28px; padding:0 12px; border-radius:999px; background:var(--accent-soft); color:var(--accent-strong); font-size:12px; font-weight:700; letter-spacing:.06em; text-transform:uppercase; }
		.header h1 { margin:18px 0 14px; font-size:46px; line-height:1.05; letter-spacing:-.04em; max-width:760px; }
		.lead { margin:0; max-width:760px; color:#393f44; font-size:19px; line-height:1.8; }
		.layout { display:grid; grid-template-columns:minmax(0,1fr) 300px; gap:18px; margin-top:18px; }
		.article-card { padding:32px; }
		.article-body { font-size:19px; line-height:1.85; color:#252b30; }
		.article-body p { margin:0 0 1.25em; }
		.article-body p:last-child { margin-bottom:0; }
		.sidebar { display:grid; gap:18px; align-content:start; }
		.side-card { padding:24px; }
		.side-card h2 { margin:0 0 12px; font-size:21px; }
		.side-card p { margin:0; color:var(--muted); line-height:1.7; }
		.meta-line { display:grid; gap:8px; color:var(--muted); font-size:14px; margin-top:16px; }
		.text-link { color:var(--accent); font-weight:700; }
		@media (max-width: 980px) {
			.layout { grid-template-columns:1fr; }
			.topbar { flex-direction:column; }
			.header h1 { font-size:38px; }
		}
		@media (max-width: 640px) {
			.shell { padding:20px 16px 54px; }
			.header, .article-card, .side-card { border-radius:24px; }
			.header, .article-card { padding:24px; }
			.header h1 { font-size:32px; }
			.article-body { font-size:18px; }
		}
	</style>
</head>
<body>
	<div class="shell">
		<div class="topbar">
			<a class="back" href="/">← На главную</a>
			<div class="actions">
				<a class="button ghost" href="/login">Войти</a>
				<a class="button solid" href="/register">Запросить доступ</a>
			</div>
		</div>

		<header class="header">
			<div class="kicker">
				<span class="tag">` + html.EscapeString(articleCategory(*article)) + `</span>
				<span>` + html.EscapeString(formatDate(article.CreatedAt)) + `</span>
				<span>` + html.EscapeString(articleReadingTimeLabel(article.Content)) + `</span>
			</div>
			<h1>` + html.EscapeString(article.Title) + `</h1>
			<p class="lead">` + articleLeadSnippet(article.Content, 280) + `</p>
		</header>

		<div class="layout">
			<article class="article-card">
				<div class="article-body">` + safeHTML(article.Content) + `</div>
			</article>
			<aside class="sidebar">
				<section class="side-card">
					<h2>Материал</h2>
					<p>Спокойный разбор для чтения без лишнего визуального шума.</p>
					<div class="meta-line">
						<span>Раздел: ` + html.EscapeString(articleCategory(*article)) + `</span>
						<span>Источник: ` + html.EscapeString(articleSourceLabel(article.Link)) + `</span>
						<span>Опубликовано: ` + html.EscapeString(formatDateTime(article.CreatedAt)) + `</span>
					</div>
				</section>
				<section class="side-card">
					<h2>Обсуждение</h2>
					<p>Комментарии и заметки доступны участникам клуба.</p>
					<div class="meta-line">
						<a class="text-link" href="` + html.EscapeString(commentURL) + `">Перейти к обсуждению</a>`)

	if article.Link != "" {
		b.WriteString(`<a class="text-link" href="` + html.EscapeString(article.Link) + `" target="_blank" rel="noreferrer">Открыть источник</a>`)
	}

	b.WriteString(`</div>
				</section>
			</aside>
		</div>
	</div>
</body>
</html>`)
	return b.String()
}

func forumThreadHTML(article *Article, commentURL string) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="ru">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>` + html.EscapeString(article.Title) + `</title>
	<style>
		:root { color-scheme: light; --bg:#edf2f7; --panel:rgba(255,255,255,.92); --ink:#152233; --muted:#677688; --line:rgba(21,34,51,.10); --accent:#125db1; --accent-dark:#0b478d; --accent-soft:#dfeefe; --shadow:0 22px 54px rgba(16,43,76,.08); }
		* { box-sizing:border-box; }
		body { margin:0; font-family:"IBM Plex Sans","Segoe UI",Tahoma,sans-serif; color:var(--ink); background:radial-gradient(circle at top right, rgba(18,93,177,.10), transparent 25%), linear-gradient(180deg,#dfe8f3 0,#edf2f7 28%,#edf2f7 100%); }
		a { color:inherit; text-decoration:none; }
		.shell { max-width:1240px; margin:0 auto; padding:24px 20px 72px; }
		.topbar { display:flex; justify-content:space-between; align-items:flex-start; gap:18px; margin-bottom:24px; }
		.back { color:#61758c; font-size:14px; letter-spacing:.04em; text-transform:uppercase; }
		.actions { display:flex; gap:12px; flex-wrap:wrap; }
		.button { display:inline-flex; align-items:center; justify-content:center; min-height:44px; padding:0 18px; border-radius:999px; font-size:15px; font-weight:700; }
		.button.solid { background:var(--accent); color:#fff; box-shadow:0 14px 30px rgba(18,93,177,.18); }
		.button.ghost { border:1px solid var(--line); background:rgba(255,255,255,.72); }
		.layout { display:grid; grid-template-columns:minmax(0,1fr) 300px; gap:18px; }
		.post-card, .side-card { background:var(--panel); border:1px solid var(--line); border-radius:28px; box-shadow:var(--shadow); }
		.post-card { padding:30px 32px; }
		.crumbs { display:flex; gap:10px; flex-wrap:wrap; align-items:center; color:#6d7f95; font-size:14px; margin-bottom:18px; }
		.post-top { display:flex; gap:12px; flex-wrap:wrap; align-items:center; color:var(--muted); font-size:14px; }
		.badge { display:inline-flex; align-items:center; min-height:28px; padding:0 12px; border-radius:999px; background:var(--accent-soft); color:var(--accent-dark); font-size:12px; font-weight:700; letter-spacing:.06em; text-transform:uppercase; }
		.post-card h1 { margin:18px 0 12px; font-size:42px; line-height:1.05; letter-spacing:-.04em; max-width:760px; }
		.post-meta { color:var(--muted); font-size:15px; line-height:1.7; }
		.post-body { margin-top:24px; font-size:18px; line-height:1.8; color:#223140; }
		.post-body p { margin:0 0 1.2em; }
		.post-body p:last-child { margin-bottom:0; }
		.sidebar { display:grid; gap:18px; align-content:start; }
		.side-card { padding:24px; }
		.side-card h2 { margin:0 0 12px; font-size:21px; }
		.side-card p { margin:0; color:var(--muted); line-height:1.7; }
		.meta-list { display:grid; gap:8px; margin-top:16px; color:var(--muted); font-size:14px; }
		.text-link { color:var(--accent); font-weight:700; }
		@media (max-width: 980px) {
			.layout { grid-template-columns:1fr; }
			.topbar { flex-direction:column; }
			.post-card h1 { font-size:36px; }
		}
		@media (max-width: 640px) {
			.shell { padding:20px 16px 54px; }
			.post-card, .side-card { border-radius:24px; }
			.post-card { padding:24px; }
			.post-card h1 { font-size:31px; }
			.post-body { font-size:17px; }
		}
	</style>
</head>
<body>
	<div class="shell">
		<div class="topbar">
			<a class="back" href="/">← К списку тем</a>
			<div class="actions">
				<a class="button ghost" href="/login">Войти</a>
				<a class="button solid" href="/register">Запросить доступ</a>
			</div>
		</div>

		<div class="layout">
			<article class="post-card">
				<div class="crumbs">
					<a href="/">Форум</a>
					<span>/</span>
					<span>` + html.EscapeString(articleCategory(*article)) + `</span>
				</div>
				<div class="post-top">
					<span class="badge">` + html.EscapeString(articleCategory(*article)) + `</span>
					<span>` + html.EscapeString(formatDate(article.CreatedAt)) + `</span>
					<span>` + html.EscapeString(articleSourceLabel(article.Link)) + `</span>
				</div>
				<h1>` + html.EscapeString(article.Title) + `</h1>
				<div class="post-meta">` + html.EscapeString(forumAuthorName(article.ID)) + ` · ` + html.EscapeString(formatDateTime(forumLastActivity(*article))) + ` · ` + fmt.Sprintf("%d ответов", forumReplyCount(*article)) + ` · ` + fmt.Sprintf("%d просмотров", forumViewCount(*article)) + `</div>
				<div class="post-body">` + safeHTML(article.Content) + `</div>
			</article>
			<aside class="sidebar">
				<section class="side-card">
					<h2>Участвовать</h2>
					<p>Ответы, подписка на тему и закладки доступны участникам клуба.</p>
					<div class="meta-list">
						<a class="text-link" href="` + html.EscapeString(commentURL) + `">Написать ответ</a>`)

	if article.Link != "" {
		b.WriteString(`<a class="text-link" href="` + html.EscapeString(article.Link) + `" target="_blank" rel="noreferrer">Открыть источник</a>`)
	}

	b.WriteString(`</div>
				</section>
				<section class="side-card">
					<h2>По теме</h2>
					<p>Ветка сохранена в разделе ` + html.EscapeString(articleCategory(*article)) + ` и остаётся доступной для чтения в архиве.</p>
				</section>
			</aside>
		</div>
	</div>
</body>
</html>`)
	return b.String()
}

func commentGateHTML(article *Article, returnURL string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="ru">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Обсуждение</title>
	<style>
		:root { color-scheme: light; --ink:#182536; --muted:#677688; --line:rgba(21,34,51,.10); --accent:#125db1; --accent-dark:#0b478d; --accent-soft:#dfeefe; }
		* { box-sizing:border-box; }
		body { margin:0; min-height:100vh; display:grid; place-items:center; padding:20px; font-family:"IBM Plex Sans","Segoe UI",Tahoma,sans-serif; color:var(--ink); background:radial-gradient(circle at top, rgba(18,93,177,.12), transparent 36%%), linear-gradient(180deg,#e5edf7 0,#f4f8fc 100%%); }
		.card { width:min(580px,100%%); background:rgba(255,255,255,.94); border:1px solid var(--line); border-radius:26px; padding:30px; box-shadow:0 22px 54px rgba(16,43,76,.10); }
		h1 { margin:0 0 12px; font-size:34px; line-height:1.08; letter-spacing:-.04em; }
		p { margin:0; color:var(--muted); line-height:1.7; }
		.actions { display:flex; gap:12px; flex-wrap:wrap; margin-top:22px; }
		.button { display:inline-flex; align-items:center; justify-content:center; min-height:44px; padding:0 18px; border-radius:999px; font-size:15px; font-weight:700; text-decoration:none; }
		.button.solid { background:var(--accent); color:#fff; }
		.button.ghost { border:1px solid var(--line); color:var(--ink); background:#fff; }
		.note { margin-top:18px; padding:14px 16px; background:#f7fbff; border:1px solid rgba(18,93,177,.12); border-radius:16px; color:#56677b; }
	</style>
</head>
<body>
	<div class="card">
		<h1>Обсуждение доступно участникам</h1>
		<p>Чтобы ответить на материал «%s», войдите в аккаунт или используйте код доступа.</p>
		<div class="note">Чтение остаётся открытым, а участие в дискуссии доступно после входа.</div>
		<div class="actions">
			<a class="button solid" href="/login">Войти</a>
			<a class="button ghost" href="%s">Вернуться к материалу</a>
		</div>
	</div>
</body>
</html>`, html.EscapeString(article.Title), html.EscapeString(returnURL))
}

func authPageHTML(title, subtitle, errMessage string, includeRegisterFields bool) string {
	var extraFields string
	var buttonLabel string
	if includeRegisterFields {
		extraFields = `<input type="text" name="username" placeholder="Имя или псевдоним" required><br>
		<input type="password" name="password" placeholder="Пароль" required><br>`
		buttonLabel = "Отправить запрос"
	} else {
		extraFields = `<input type="text" name="username" placeholder="Логин или e-mail" required><br>
		<input type="password" name="password" placeholder="Пароль" required><br>`
		buttonLabel = "Продолжить"
	}

	var msg string
	if errMessage != "" {
		msg = `<div class="msg">` + html.EscapeString(errMessage) + `</div>`
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="ru">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>%s</title>
	<style>
		:root { color-scheme: light; --ink:#182536; --muted:#677688; --line:rgba(21,34,51,.10); --accent:#125db1; --accent-dark:#0b478d; --accent-soft:#dfeefe; }
		* { box-sizing:border-box; }
		body { margin:0; min-height:100vh; display:grid; place-items:center; padding:20px; font-family:"IBM Plex Sans","Segoe UI",Tahoma,sans-serif; color:var(--ink); background:radial-gradient(circle at top, rgba(18,93,177,.12), transparent 36%%), linear-gradient(180deg,#e5edf7 0,#f4f8fc 100%%); }
		.card { width:min(480px,100%%); background:rgba(255,255,255,.94); border:1px solid var(--line); border-radius:26px; padding:30px; box-shadow:0 22px 54px rgba(16,43,76,.10); }
		h1 { margin:0 0 10px; font-size:34px; line-height:1.08; letter-spacing:-.04em; }
		p { color:var(--muted); line-height:1.7; }
		input { width:100%%; padding:13px 15px; margin:10px 0 0; border-radius:14px; border:1px solid rgba(21,34,51,.12); background:#fcfdff; font:inherit; }
		button { margin-top:16px; width:100%%; min-height:46px; padding:0 16px; border:none; border-radius:14px; background:var(--accent); color:#fff; font:inherit; font-weight:700; cursor:pointer; }
		.note { margin-top:18px; font-size:14px; color:#6f8196; }
		.msg { margin:18px 0; padding:12px 14px; background:#f7fbff; border:1px solid rgba(18,93,177,.12); border-radius:14px; color:#5a6e84; }
		a { color:var(--accent); text-decoration:none; font-weight:700; }
	</style>
</head>
<body>
	<div class="card">
		<p><a href="/">← Вернуться на сайт</a></p>
		<h1>%s</h1>
		<p>%s</p>
		%s
		<form method="POST">
			%s
			<input type="text" name="invite_code" placeholder="Код доступа" required><br>
			<button type="submit">%s</button>
		</form>
		<div class="note">Если кода пока нет, дождитесь приглашения или запроса от администратора клуба.</div>
	</div>
</body>
</html>`, title, title, subtitle, msg, extraFields, buttonLabel)
}

func formatDate(t time.Time) string {
	if t.IsZero() {
		return "недавно"
	}
	months := []string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}
	month := months[0]
	if monthIndex := int(t.Month()) - 1; monthIndex >= 0 && monthIndex < len(months) {
		month = months[monthIndex]
	}
	return fmt.Sprintf("%d %s %d", t.Day(), month, t.Year())
}

func safeSnippet(s string, limit int) string {
	s = normalizedContentText(s)
	s = html.EscapeString(s)
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "..."
}

func safeHTML(s string) string {
	paragraphs := contentParagraphs(s)
	if len(paragraphs) == 0 {
		return `<p>Материал обновляется.</p>`
	}

	var b strings.Builder
	for _, paragraph := range paragraphs {
		fmt.Fprintf(&b, `<p>%s</p>`, html.EscapeString(paragraph))
	}
	return b.String()
}

func formatDateTime(t time.Time) string {
	if t.IsZero() {
		return "недавно"
	}
	return fmt.Sprintf("%s, %02d:%02d", formatDate(t), t.Hour(), t.Minute())
}

func articleLeadSnippet(content string, limit int) string {
	paragraphs := contentParagraphs(content)
	if len(paragraphs) == 0 {
		return safeSnippet(content, limit)
	}
	return safeSnippet(paragraphs[0], limit)
}

func articleReadingTimeLabel(content string) string {
	words := len(strings.Fields(normalizedContentText(content)))
	if words == 0 {
		return "3 мин"
	}

	minutes := words / 180
	if words%180 != 0 {
		minutes++
	}
	if minutes < 3 {
		minutes = 3
	}
	return fmt.Sprintf("%d мин", minutes)
}

func articleCategory(article Article) string {
	text := strings.ToLower(article.Title + " " + article.Content)

	switch {
	case strings.Contains(text, "tls"), strings.Contains(text, "ssl"), strings.Contains(text, "https"), strings.Contains(text, "сертифик"), strings.Contains(text, "шифр"):
		return "Безопасность"
	case strings.Contains(text, "dns"), strings.Contains(text, "bgp"), strings.Contains(text, "tcp"), strings.Contains(text, "udp"), strings.Contains(text, "маршрут"), strings.Contains(text, "routing"), strings.Contains(text, "сеть"), strings.Contains(text, "сетев"):
		return "Сети"
	case strings.Contains(text, "docker"), strings.Contains(text, "kubernetes"), strings.Contains(text, "nginx"), strings.Contains(text, "сервер"), strings.Contains(text, "devops"), strings.Contains(text, "ci/cd"), strings.Contains(text, "инфраструкт"):
		return "Инфраструктура"
	case strings.Contains(text, "postgres"), strings.Contains(text, "mysql"), strings.Contains(text, "sqlite"), strings.Contains(text, "clickhouse"), strings.Contains(text, "sql"), strings.Contains(text, "база данных"):
		return "Данные"
	case strings.Contains(text, "golang"), strings.Contains(text, "go "), strings.Contains(text, "python"), strings.Contains(text, "rust"), strings.Contains(text, "код"), strings.Contains(text, "разработ"):
		return "Разработка"
	case strings.Contains(text, "self-host"), strings.Contains(text, "самохост"), strings.Contains(text, "homelab"), strings.Contains(text, "домашн"), strings.Contains(text, "nas"):
		return "Самохостинг"
	default:
		return "Практика"
	}
}

func articleSourceLabel(link string) string {
	host := articleSourceHost(link)
	if host == "" {
		return "редакционная подборка"
	}
	return host
}

func articleSourceHost(link string) string {
	if strings.TrimSpace(link) == "" {
		return ""
	}

	parsed, err := url.Parse(link)
	if err != nil {
		return ""
	}

	host := strings.TrimSpace(parsed.Hostname())
	host = strings.TrimPrefix(host, "www.")
	return host
}

func contentParagraphs(s string) []string {
	s = strings.TrimSpace(normalizeContentBreaks(s))
	if s == "" {
		return nil
	}

	rawParagraphs := strings.Split(s, "\n\n")
	paragraphs := make([]string, 0, len(rawParagraphs))
	for _, rawParagraph := range rawParagraphs {
		lines := strings.Split(rawParagraph, "\n")
		cleanedLines := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.Join(strings.Fields(line), " ")
			if line != "" {
				cleanedLines = append(cleanedLines, line)
			}
		}
		if len(cleanedLines) == 0 {
			continue
		}
		paragraphs = append(paragraphs, strings.Join(cleanedLines, " "))
	}
	return paragraphs
}

func normalizedContentText(s string) string {
	paragraphs := contentParagraphs(s)
	return strings.Join(paragraphs, " ")
}

func normalizeContentBreaks(s string) string {
	replacer := strings.NewReplacer(
		"\r\n", "\n",
		"\r", "\n",
		"<br />", "\n",
		"<br/>", "\n",
		"<br>", "\n",
	)
	s = replacer.Replace(strings.TrimSpace(s))

	var out []string
	prevBlank := false
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if prevBlank {
				continue
			}
			out = append(out, "")
			prevBlank = true
			continue
		}
		out = append(out, trimmed)
		prevBlank = false
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func blogSectionChipsHTML(articles []Article, limit int) string {
	categories := topCategories(articles, limit)
	if len(categories) == 0 {
		categories = []string{"Сети", "Инфраструктура", "Практика"}
	}

	var b strings.Builder
	for _, category := range categories {
		fmt.Fprintf(&b, `<span>%s</span>`, html.EscapeString(category))
	}
	return b.String()
}

func blogSidebarHTML(articles []Article) string {
	var b strings.Builder
	b.WriteString(`<section class="side-card">
			<h3>О журнале</h3>
			<p>Короткие и спокойные материалы о сетях, инфраструктуре и самостоятельных сервисах.</p>
		</section>
		<section class="side-card">
			<h3>Разделы</h3>
			<div class="chips">`)
	b.WriteString(blogSectionChipsHTML(articles, 6))
	b.WriteString(`</div>
		</section>`)

	if len(articles) > 0 {
		b.WriteString(`<section class="side-card">
				<h3>Свежие материалы</h3>
				<ul class="mini-list">`)
		for index, article := range articles {
			if index >= 4 {
				break
			}
			fmt.Fprintf(&b, `<li><a href="/article/%d">%s</a><span>%s · %s</span></li>`,
				article.ID,
				html.EscapeString(article.Title),
				html.EscapeString(articleCategory(article)),
				html.EscapeString(formatDate(article.CreatedAt)))
		}
		b.WriteString(`</ul>
			</section>`)
	}

	b.WriteString(`<section class="side-card">
			<h3>Клуб читателей</h3>
			<p>Комментарии, закладки и обсуждения доступны участникам клуба.</p>
			<div class="info-line">
				<a class="text-link" href="/register">Запросить доступ</a>
			</div>
		</section>`)

	return b.String()
}

func forumSidebarHTML(articles []Article) string {
	var b strings.Builder
	b.WriteString(`<section class="side-card">
			<h3>Разделы</h3>
			<div class="chips">`)
	b.WriteString(blogSectionChipsHTML(articles, 6))
	b.WriteString(`</div>
		</section>`)

	if len(articles) > 0 {
		b.WriteString(`<section class="side-card">
				<h3>Свежие обсуждения</h3>
				<ul class="mini-list">`)
		for index, article := range articles {
			if index >= 4 {
				break
			}
			fmt.Fprintf(&b, `<li><a href="/thread/%d">%s</a><span>%d ответов · %s</span></li>`,
				article.ID,
				html.EscapeString(article.Title),
				forumReplyCount(article),
				html.EscapeString(formatDate(forumLastActivity(article))))
		}
		b.WriteString(`</ul>
			</section>`)
	}

	b.WriteString(`<section class="side-card">
			<h3>Доступ</h3>
			<p>Чтение остаётся открытым, а ответы и подписка на темы доступны участникам клуба.</p>
		</section>`)

	return b.String()
}

func topCategories(articles []Article, limit int) []string {
	if limit <= 0 {
		return nil
	}

	seen := make(map[string]struct{})
	categories := make([]string, 0, limit)
	for _, article := range articles {
		category := articleCategory(article)
		if _, ok := seen[category]; ok {
			continue
		}
		seen[category] = struct{}{}
		categories = append(categories, category)
		if len(categories) >= limit {
			break
		}
	}
	return categories
}

func articleInitial(article Article) string {
	category := articleCategory(article)
	if category == "" {
		return "Т"
	}
	initials := []rune(category)
	if len(initials) == 0 {
		return "Т"
	}
	return strings.ToUpper(string(initials[0]))
}

func forumAuthorName(id int) string {
	authors := []string{
		"Максим С.",
		"Ирина Л.",
		"Павел К.",
		"Анна Р.",
		"Дмитрий В.",
		"Елена Т.",
		"Никита Ф.",
		"Мария Г.",
	}
	if id <= 0 {
		return authors[0]
	}
	return authors[(id-1)%len(authors)]
}

func forumReplyCount(article Article) int {
	lengthScore := len([]rune(normalizedContentText(article.Content))) / 95
	replies := 6 + (article.ID*3+lengthScore)%27
	if replies < 8 {
		replies = 8
	}
	return replies
}

func forumViewCount(article Article) int {
	return 140 + forumReplyCount(article)*19 + article.ID*13
}

func forumLastActivity(article Article) time.Time {
	if article.CreatedAt.IsZero() {
		return time.Time{}
	}
	offsetMinutes := 35 + (article.ID*17)%180
	return article.CreatedAt.Add(time.Duration(offsetMinutes) * time.Minute)
}

func forumTotalReplies(articles []Article) int {
	total := 0
	for _, article := range articles {
		total += forumReplyCount(article)
	}
	return total
}

func forumMemberCount(articles []Article) int {
	return 84 + len(articles)*17
}
