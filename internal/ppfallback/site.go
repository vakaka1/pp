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
	siteHints  FallbackSiteHints
}

type FallbackSiteHints struct {
	Domain   string
	Keywords []string
}

func NewFallbackHandler(fallbackType, proxyAddress, inviteCode string, db *FallbackDB, hints ...FallbackSiteHints) (*FallbackHandler, error) {
	siteHints := FallbackSiteHints{}
	if len(hints) > 0 {
		siteHints = normalizeFallbackSiteHints(hints[0])
	}

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
			siteHints:  siteHints,
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
		siteHints:  siteHints,
	}, nil
}

func normalizeFallbackSiteHints(hints FallbackSiteHints) FallbackSiteHints {
	out := FallbackSiteHints{
		Domain: strings.TrimSpace(hints.Domain),
	}
	seen := make(map[string]struct{})
	for _, keyword := range hints.Keywords {
		keyword = strings.Join(strings.Fields(keyword), " ")
		if keyword == "" {
			continue
		}
		key := strings.ToLower(keyword)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out.Keywords = append(out.Keywords, keyword)
	}
	return out
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
		fmt.Fprint(w, forumIndexHTML(articles, h.siteHints))
		return
	}
	fmt.Fprint(w, blogIndexHTML(articles, h.siteHints))
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
			errMessage = "Для регистрации нужен код приглашения."
		} else if h.inviteCode != "" && invite != h.inviteCode {
			errMessage = "Код приглашения не найден. Проверьте написание или обратитесь к администратору."
		} else {
			errMessage = "Регистрация временно недоступна. Попробуйте позже."
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, authPageHTML("Регистрация", "Новый аккаунт создаётся после проверки кода приглашения.", errMessage, true, true))
}

func (h *FallbackHandler) serveLogin(w http.ResponseWriter, r *http.Request) {
	errMessage := ""
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		username := strings.TrimSpace(r.FormValue("username"))
		password := strings.TrimSpace(r.FormValue("password"))
		if username == "" || password == "" {
			errMessage = "Введите логин и пароль."
		} else {
			errMessage = "Сервис входа временно недоступен. Повторите попытку позже."
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, authPageHTML("Вход", "Для комментариев и закрытых разделов используйте данные своей учётной записи.", errMessage, false, false))
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

func blogIndexHTML(articles []Article, hints FallbackSiteHints) string {
	var b strings.Builder
	profile := blogSiteProfile(articles, hints)
	b.WriteString(`<!DOCTYPE html>
<html lang="ru">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>` + html.EscapeString(profile.Title) + `</title>
	<style>
		:root { color-scheme: light; --paper:#f4f0e8; --panel:#fffdfa; --ink:#23201b; --muted:#746b5f; --line:rgba(68,54,39,.16); --accent:#8f4b2f; --accent-dark:#62311f; --green:#4f6f61; --shadow:0 16px 42px rgba(80,61,38,.09); }
		* { box-sizing:border-box; }
		body { margin:0; font-family:"Literata","Iowan Old Style","Palatino Linotype",Georgia,serif; color:var(--ink); background:linear-gradient(180deg,#eee6da 0,#f4f0e8 38%,#f7f4ee 100%); }
		a { color:inherit; text-decoration:none; }
		.page { max-width:1160px; margin:0 auto; padding:26px 20px 76px; }
		.masthead { display:grid; grid-template-columns:minmax(0,1fr) auto; gap:24px; align-items:end; padding-bottom:22px; border-bottom:1px solid var(--line); }
		.brand-word { display:inline-block; font-size:64px; line-height:.92; font-weight:800; }
		.brand-word::after { content:""; display:block; width:84px; height:6px; margin-top:14px; border-radius:999px; background:linear-gradient(90deg,var(--accent),rgba(79,111,97,.22)); }
		.masthead p { margin:18px 0 0; max-width:650px; color:#5f554b; font-size:18px; line-height:1.65; }
		.nav { display:flex; gap:8px; flex-wrap:wrap; justify-content:flex-end; }
		.nav a, .button { display:inline-flex; align-items:center; justify-content:center; min-height:40px; padding:0 15px; border-radius:999px; border:1px solid var(--line); background:rgba(255,253,250,.72); color:#3c352e; font-size:14px; font-weight:700; }
		.button.solid { border-color:transparent; background:var(--accent); color:#fff8f1; box-shadow:0 13px 28px rgba(105,52,30,.18); }
		.top-feed { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); gap:16px; margin-top:26px; align-items:stretch; }
		.lead-card, .post-row { background:rgba(255,253,250,.92); border:1px solid var(--line); border-radius:8px; box-shadow:var(--shadow); }
		.lead-card { min-height:280px; padding:28px; display:flex; flex-direction:column; justify-content:space-between; }
		.lead-card.empty { grid-column:1 / -1; min-height:220px; }
		.meta { display:flex; gap:10px; flex-wrap:wrap; align-items:center; color:var(--muted); font-size:14px; }
		.lead-card h2 { margin:18px 0 12px; font-size:30px; line-height:1.14; }
		.lead-card p { margin:0; color:#3f3831; font-size:17px; line-height:1.75; }
		.card-foot { display:flex; justify-content:space-between; gap:14px; flex-wrap:wrap; margin-top:24px; padding-top:16px; border-top:1px solid rgba(68,54,39,.13); color:var(--muted); font-size:14px; }
		.archive { margin-top:28px; }
		.archive-head { display:flex; justify-content:space-between; gap:14px; align-items:end; margin:0 0 14px; padding:0 4px; }
		.archive-head h2 { margin:0; font-size:28px; }
		.archive-head span { color:var(--muted); font-size:14px; }
		.feed { display:grid; gap:14px; }
		.post-row { display:grid; grid-template-columns:128px minmax(0,1fr); gap:22px; padding:24px 26px; }
		.post-date { color:var(--green); font-weight:800; line-height:1.35; }
		.post-row h2 { margin:9px 0 10px; font-size:28px; line-height:1.16; }
		.post-row p { margin:0; color:#453d35; line-height:1.78; }
		.read-more { display:inline-flex; margin-top:16px; color:var(--accent-dark); font-weight:800; }
		.empty-note { padding:24px; border-radius:8px; border:1px dashed rgba(68,54,39,.22); color:var(--muted); background:rgba(255,253,250,.68); line-height:1.7; }
		@media (max-width: 980px) {
			.masthead { grid-template-columns:1fr; }
			.nav { justify-content:flex-start; }
			.top-feed { grid-template-columns:repeat(2,minmax(0,1fr)); }
		}
		@media (max-width: 640px) {
			.page { padding:20px 16px 54px; }
			.brand-word { font-size:46px; }
			.top-feed { grid-template-columns:1fr; }
			.lead-card { min-height:0; padding:24px; }
			.lead-card h2 { font-size:26px; }
			.post-row { grid-template-columns:1fr; padding:22px; }
			.post-row h2 { font-size:25px; }
		}
	</style>
</head>
<body>
	<div class="page">
		<header class="masthead">
			<div>
				<a class="brand-word" href="/">` + html.EscapeString(profile.Title) + `</a>
				<p>` + html.EscapeString(profile.Subtitle) + `</p>
			</div>
			<nav class="nav" aria-label="Навигация">
				<a href="/">Лента</a>
				<a href="#archive">Архив</a>
				<a href="/login">Войти</a>
				<a class="button solid" href="/register">Регистрация</a>
			</nav>
		</header>

		<section class="top-feed" aria-label="Последние записи">`)

	if len(articles) == 0 {
		b.WriteString(`<article class="lead-card empty">
				<div>
					<div class="meta"><span>скоро</span></div>
					<h2>Здесь пока тихо</h2>
					<p>Первые записи ещё готовятся. Когда в ленте появятся материалы, здесь будут последние публикации и аккуратный архив.</p>
				</div>
				<div class="card-foot">
					<span>новая лента</span>
					<span>архив готовится</span>
				</div>
			</article>`)
	} else {
		limit := len(articles)
		if limit > 3 {
			limit = 3
		}
		for _, article := range articles[:limit] {
			fmt.Fprintf(&b, `<article class="lead-card">
				<div>
					<div class="meta">
						<span>%s</span>
						<span>%s</span>
					</div>
					<h2><a href="/article/%d">%s</a></h2>
					<p>%s</p>
				</div>
				<div class="card-foot">
					<span>%s</span>
					<a class="read-more" href="/article/%d">читать дальше</a>
				</div>
			</article>`,
				html.EscapeString(formatDate(article.CreatedAt)),
				html.EscapeString(articleReadingTimeLabel(article.Content)),
				article.ID,
				html.EscapeString(article.Title),
				safeSnippet(article.Content, 240),
				html.EscapeString(formatDateTime(article.CreatedAt)),
				article.ID)
		}
	}

	b.WriteString(`</section>

		<section class="archive" id="archive">
			<div class="archive-head">
				<h2>Архив</h2>
				<span>`)
	b.WriteString(html.EscapeString(articleCountLabel(len(articles))))
	b.WriteString(`</span>
			</div>
			<div class="feed">`)

	if len(articles) <= 3 {
		b.WriteString(`<div class="empty-note">Новые записи появятся здесь по мере публикации. Архив пополняется постепенно и без отдельного расписания.</div>`)
	} else {
		for _, a := range articles[3:] {
			fmt.Fprintf(&b, `<article class="post-row">
					<div class="post-date">%s</div>
					<div>
						<div class="meta">
							<span>%s</span>
						</div>
						<h2><a href="/article/%d">%s</a></h2>
						<p>%s</p>
						<a class="read-more" href="/article/%d">читать дальше</a>
					</div>
				</article>`,
				html.EscapeString(formatDate(a.CreatedAt)),
				html.EscapeString(articleReadingTimeLabel(a.Content)),
				a.ID,
				html.EscapeString(a.Title),
				safeSnippet(a.Content, 230),
				a.ID)
		}
	}

	b.WriteString(`</div>
		</section>
	</div>
</body>
</html>`)
	return b.String()
}

func forumIndexHTML(articles []Article, hints FallbackSiteHints) string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="ru">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Форум обсуждений</title>
	<style>
		:root { color-scheme: light; --bg:#edf2f7; --panel:rgba(255,255,255,.92); --ink:#152233; --muted:#677688; --line:rgba(21,34,51,.10); --accent:#125db1; --accent-dark:#0b478d; --accent-soft:#dfeefe; --shadow:0 22px 54px rgba(16,43,76,.08); }
		* { box-sizing:border-box; }
		body { margin:0; font-family:"IBM Plex Sans","Segoe UI",Tahoma,sans-serif; color:var(--ink); background:radial-gradient(circle at top right, rgba(18,93,177,.10), transparent 25%), linear-gradient(180deg,#dfe8f3 0,#edf2f7 28%,#edf2f7 100%); }
		a { color:inherit; text-decoration:none; }
		.shell { max-width:1240px; margin:0 auto; padding:24px 20px 72px; }
		.topbar { display:flex; justify-content:space-between; align-items:flex-start; gap:18px; margin-bottom:20px; }
		.brand small { display:inline-block; margin-bottom:12px; font-size:12px; letter-spacing:.18em; text-transform:uppercase; color:#6d7f95; }
		.brand h1 { margin:0; font-size:46px; line-height:1; letter-spacing:-.04em; }
		.brand p { margin:10px 0 0; max-width:620px; color:var(--muted); line-height:1.65; font-size:17px; }
		.actions { display:flex; gap:12px; flex-wrap:wrap; }
		.button { display:inline-flex; align-items:center; justify-content:center; min-height:44px; padding:0 18px; border-radius:999px; font-size:15px; font-weight:700; transition:.2s ease; }
		.button.solid { background:var(--accent); color:#fff; box-shadow:0 14px 30px rgba(18,93,177,.18); }
		.button.ghost { border:1px solid var(--line); background:rgba(255,255,255,.72); color:var(--ink); }
		.layout { display:grid; grid-template-columns:minmax(0,1fr) 310px; gap:18px; }
		.board, .side-card, .post-card { background:var(--panel); border:1px solid var(--line); border-radius:26px; box-shadow:var(--shadow); }
		.board { overflow:hidden; }
		.board-head, .board-row { display:grid; grid-template-columns:minmax(0,1.8fr) 94px 190px; gap:16px; align-items:center; padding:18px 22px; }
		.board-head { background:#f8fbff; color:#5f7389; font-size:13px; font-weight:700; letter-spacing:.08em; text-transform:uppercase; }
		.board-row { border-top:1px solid var(--line); }
		.topic-cell { display:grid; grid-template-columns:54px minmax(0,1fr); gap:14px; align-items:start; }
		.avatar { width:54px; height:54px; border-radius:16px; display:grid; place-items:center; background:linear-gradient(135deg,#1a67bd,#0b478d); color:#fff; font-size:20px; font-weight:700; }
		.topic-top { display:flex; gap:10px; flex-wrap:wrap; align-items:center; margin-bottom:8px; }
		.badge { display:inline-flex; align-items:center; min-height:26px; padding:0 10px; border-radius:999px; background:var(--accent-soft); color:var(--accent-dark); font-size:12px; font-weight:700; letter-spacing:.06em; text-transform:uppercase; }
		.topic-title { display:block; font-size:21px; font-weight:700; line-height:1.25; letter-spacing:-.02em; }
		.topic-copy { margin:8px 0 0; color:var(--muted); line-height:1.65; }
		.topic-meta { margin-top:10px; color:var(--muted); font-size:14px; }
		.count { color:#223449; font-weight:700; }
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
			.board-head { display:none; }
			.board-row { grid-template-columns:1fr; gap:14px; }
			.count::before { content:"Ответы: "; color:var(--muted); font-weight:600; }
			.last::before { content:"Обновлено: "; color:var(--muted); font-weight:600; }
		}
		@media (max-width: 640px) {
			.shell { padding:20px 16px 54px; }
			.brand h1 { font-size:38px; }
			.board, .side-card, .post-card { border-radius:22px; }
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
				<small>Форум</small>
				<h1><a href="/">Форум обсуждений</a></h1>
				<p>` + html.EscapeString(forumSubtitle(articles, hints)) + `</p>
			</div>
			<div class="actions">
				<a class="button ghost" href="/login">Войти</a>
				<a class="button solid" href="/register">Регистрация</a>
			</div>
		</header>

		<div class="layout">
			<section class="board">
				<div class="board-head">
					<div>Тема</div>
					<div>Ответы</div>
					<div>Последнее</div>
				</div>`)

	if len(articles) == 0 {
		b.WriteString(`<div class="empty">Список тем пока пуст. Первые обсуждения появятся после публикации новых заметок.</div>`)
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
					<div class="count">%d</div>
					<div class="last">%s</div>
				</div>`,
				html.EscapeString(articleInitial(a)),
				html.EscapeString(articleCategory(a)),
				html.EscapeString(articleReadingTimeLabel(a.Content)),
				a.ID,
				html.EscapeString(a.Title),
				safeSnippet(a.Content, 150),
				html.EscapeString(forumAuthorName(a.ID)),
				html.EscapeString(formatDate(a.CreatedAt)),
				forumReplyCount(a),
				html.EscapeString(formatDateTime(forumLastActivity(a))))
		}
	}

	b.WriteString(`</section>
			<aside class="sidebar">`)
	b.WriteString(forumSidebarHTML(articles, hints))
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
		:root { color-scheme: light; --paper:#f4f0e8; --panel:#fffdfa; --ink:#23201b; --muted:#746b5f; --line:rgba(68,54,39,.16); --accent:#8f4b2f; --accent-dark:#62311f; --accent-soft:#ead8ca; --green:#4f6f61; --shadow:0 16px 42px rgba(80,61,38,.09); }
		* { box-sizing:border-box; }
		body { margin:0; font-family:"Literata","Iowan Old Style","Palatino Linotype",Georgia,serif; color:var(--ink); background:linear-gradient(180deg,#eee6da 0,#f4f0e8 38%,#f7f4ee 100%); }
		a { color:inherit; text-decoration:none; }
		.page { max-width:1120px; margin:0 auto; padding:24px 20px 76px; }
		.article-nav { display:flex; justify-content:space-between; gap:16px; flex-wrap:wrap; align-items:center; padding-bottom:18px; border-bottom:1px solid var(--line); color:#5a5047; font-size:14px; font-weight:800; }
		.nav-links { display:flex; gap:8px; flex-wrap:wrap; }
		.nav-links a { display:inline-flex; align-items:center; min-height:38px; padding:0 14px; border:1px solid var(--line); border-radius:999px; background:rgba(255,253,250,.72); }
		.hero, .prose, .side-card { background:rgba(255,253,250,.92); border:1px solid var(--line); border-radius:8px; box-shadow:var(--shadow); }
		.hero { position:relative; overflow:hidden; max-width:940px; margin:24px auto 0; padding:40px; }
		.kicker { position:relative; display:flex; gap:10px; flex-wrap:wrap; align-items:center; color:var(--muted); font-size:14px; }
		.hero h1 { position:relative; max-width:820px; margin:22px 0 16px; font-size:52px; line-height:1.02; }
		.byline { display:flex; gap:12px; flex-wrap:wrap; margin-top:30px; padding-top:18px; border-top:1px solid rgba(68,54,39,.13); color:var(--muted); font-size:14px; }
		.article-layout { max-width:900px; margin:20px auto 0; }
		.article-main { display:grid; gap:18px; min-width:0; }
		.prose { padding:42px; }
		.article-body { font-size:20px; line-height:1.9; color:#2e2923; }
		.article-body p { margin:0 0 1.35em; }
		.article-body p:first-child::first-letter { float:left; margin:.1em .12em 0 0; font-size:4.1em; line-height:.74; color:var(--accent-dark); font-weight:800; }
		.article-body p:last-child { margin-bottom:0; }
		.article-body a { color:var(--accent-dark); font-weight:700; text-decoration:underline; text-decoration-thickness:1px; text-underline-offset:.18em; }
		.article-body h2, .article-body h3, .article-body h4 { margin:1.7em 0 .7em; line-height:1.18; color:#241f1a; }
		.article-body h2:first-child, .article-body h3:first-child, .article-body h4:first-child { margin-top:0; }
		.article-body h2 { font-size:1.55em; }
		.article-body h3 { font-size:1.28em; }
		.article-body h4 { font-size:1.12em; }
		.article-body ul, .article-body ol { margin:0 0 1.35em; padding-left:1.4em; }
		.article-body li { margin:.35em 0; }
		.article-body blockquote { margin:1.6em 0; padding:2px 0 2px 20px; border-left:4px solid rgba(143,75,47,.38); color:#4a4139; }
		.article-body blockquote p:last-child { margin-bottom:0; }
		.article-body code { font-family:"SFMono-Regular","Cascadia Code","Liberation Mono",Consolas,monospace; font-size:.88em; border-radius:5px; padding:.12em .34em; background:#efe5d8; color:#2d2822; }
		.article-body pre { margin:1.7em 0; padding:18px 20px; overflow:auto; border-radius:8px; background:#1f2933; color:#f7fafc; border:1px solid rgba(35,32,27,.18); line-height:1.62; }
		.article-body pre code { display:block; padding:0; background:transparent; color:inherit; white-space:pre; font-size:14px; }
		.article-body figure { margin:2em 0; }
		.article-body figure:first-child { margin-top:0; }
		.article-body img { display:block; width:100%; max-width:100%; height:auto; border-radius:8px; border:1px solid rgba(68,54,39,.14); box-shadow:0 18px 46px rgba(80,61,38,.12); background:#efe5d5; }
		.article-body figcaption { margin-top:10px; color:var(--muted); font-size:14px; line-height:1.5; text-align:center; }
		.side-card { padding:24px; }
		.comment-card { padding:28px; }
		.side-card h2 { margin:0 0 12px; font-size:22px; line-height:1.1; }
		.side-card p { margin:0; color:var(--muted); line-height:1.7; }
		.note-line { display:grid; gap:8px; color:var(--muted); font-size:14px; margin-top:16px; }
		.text-link { color:var(--accent-dark); font-weight:800; }
		@media (max-width: 980px) {
			.hero h1 { font-size:42px; }
		}
		@media (max-width: 640px) {
			.page { padding:20px 16px 54px; }
			.hero, .prose { padding:26px; }
			.hero h1 { font-size:34px; }
			.article-body { font-size:18px; }
			.article-body p:first-child::first-letter { float:none; margin:0; font-size:inherit; line-height:inherit; color:inherit; font-weight:inherit; }
		}
	</style>
</head>
<body>
	<div class="page">
		<nav class="article-nav" aria-label="Навигация">
			<a href="/">← на главную</a>
			<div class="nav-links">
				<a href="/">лента</a>
				<a href="/login">войти</a>
			</div>
		</nav>

		<header class="hero">
			<div class="kicker">
				<span>` + html.EscapeString(formatDate(article.CreatedAt)) + `</span>
				<span>` + html.EscapeString(articleReadingTimeLabel(article.Content)) + `</span>
			</div>
			<h1>` + html.EscapeString(article.Title) + `</h1>
			<div class="byline">
				<span>` + html.EscapeString(formatDateTime(article.CreatedAt)) + `</span>
			</div>
		</header>

		<div class="article-layout">
			<main class="article-main">
				<article class="prose">
					<div class="article-body">` + safeHTML(article.Content, article.Title) + `</div>
				</article>
				<section class="side-card comment-card">
					<h2>Обсуждение</h2>
					<p>Комментарии открыты для зарегистрированных читателей. Ответы остаются рядом с записью и не смешиваются с общей лентой.</p>
					<div class="note-line">
						<a class="text-link" href="` + html.EscapeString(commentURL) + `">Перейти к обсуждению</a>
					</div>
				</section>
			</main>
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
		.post-body h2, .post-body h3, .post-body h4 { margin:1.5em 0 .65em; line-height:1.2; }
		.post-body ul, .post-body ol { margin:0 0 1.2em; padding-left:1.35em; }
		.post-body li { margin:.3em 0; }
		.post-body blockquote { margin:1.4em 0; padding-left:18px; border-left:4px solid rgba(18,93,177,.28); color:#445568; }
		.post-body code { font-family:"SFMono-Regular","Cascadia Code","Liberation Mono",Consolas,monospace; font-size:.9em; border-radius:5px; padding:.1em .32em; background:#e9f1fb; color:#182536; }
		.post-body pre { margin:1.5em 0; padding:16px 18px; overflow:auto; border-radius:8px; background:#152233; color:#f8fbff; line-height:1.6; }
		.post-body pre code { display:block; padding:0; background:transparent; color:inherit; white-space:pre; font-size:14px; }
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
				<a class="button solid" href="/register">Регистрация</a>
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
					<span>` + html.EscapeString(articleReadingTimeLabel(article.Content)) + `</span>
				</div>
				<h1>` + html.EscapeString(article.Title) + `</h1>
				<div class="post-meta">` + html.EscapeString(forumAuthorName(article.ID)) + ` · ` + html.EscapeString(formatDateTime(forumLastActivity(*article))) + ` · ` + fmt.Sprintf("%d ответов", forumReplyCount(*article)) + `</div>
				<div class="post-body">` + safeHTML(article.Content, article.Title) + `</div>
			</article>
			<aside class="sidebar">
				<section class="side-card">
					<h2>Ответить</h2>
					<p>Чтобы написать сообщение в этой теме, войдите в аккаунт.</p>
					<div class="meta-list">
						<a class="text-link" href="` + html.EscapeString(commentURL) + `">Написать ответ</a>`)

	b.WriteString(`</div>
				</section>
				<section class="side-card">
					<h2>Раздел</h2>
					<p>Тема опубликована в разделе ` + html.EscapeString(articleCategory(*article)) + ` и остаётся доступной в архиве.</p>
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
		<h1>Чтобы оставить комментарий, войдите в аккаунт</h1>
		<p>Ответ к материалу «%s» можно отправить после входа под своей учётной записью.</p>
		<div class="note">Чтение страницы остаётся открытым, а публикация ответа доступна после входа.</div>
		<div class="actions">
			<a class="button solid" href="/login">Войти</a>
			<a class="button ghost" href="%s">Вернуться к материалу</a>
		</div>
	</div>
</body>
</html>`, html.EscapeString(article.Title), html.EscapeString(returnURL))
}

func authPageHTML(title, subtitle, errMessage string, includeRegisterFields bool, includeAccessCode bool) string {
	var extraFields string
	var buttonLabel string
	var note string
	if includeRegisterFields {
		extraFields = `<input type="text" name="username" placeholder="Имя или псевдоним" required><br>
		<input type="password" name="password" placeholder="Пароль" required><br>`
		buttonLabel = "Зарегистрироваться"
		note = "Если у вас нет кода приглашения, обратитесь к администратору сайта."
	} else {
		extraFields = `<input type="text" name="username" placeholder="Логин или e-mail" required><br>
		<input type="password" name="password" placeholder="Пароль" required><br>`
		buttonLabel = "Войти"
		note = "Если вы забыли пароль или не можете войти, обратитесь к администратору."
	}

	var msg string
	if errMessage != "" {
		msg = `<div class="msg">` + html.EscapeString(errMessage) + `</div>`
	}

	var accessField string
	if includeAccessCode {
		accessField = `<input type="text" name="invite_code" placeholder="Код приглашения" required><br>`
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
			%s
			<button type="submit">%s</button>
		</form>
		<div class="note">%s</div>
	</div>
</body>
</html>`, title, title, subtitle, msg, extraFields, accessField, buttonLabel, note)
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

func safeHTML(s string, omittedLeadingTitles ...string) string {
	blocks := contentBlocks(s)
	if len(blocks) == 0 {
		return `<p>Материал обновляется.</p>`
	}

	blocks = dropLeadingTitleBlocks(blocks, omittedLeadingTitles...)
	if len(blocks) == 0 {
		return `<p>Материал обновляется.</p>`
	}

	var b strings.Builder
	for _, block := range blocks {
		if language, code, ok := parseMarkdownCodeBlock(block); ok {
			classAttr := ""
			if language != "" {
				classAttr = ` class="language-` + html.EscapeString(language) + `"`
			}
			fmt.Fprintf(&b, `<pre><code%s>%s</code></pre>`, classAttr, html.EscapeString(code))
			continue
		}
		if src, alt, ok := parseMarkdownImageBlock(block); ok {
			fmt.Fprintf(&b, `<figure><img src="%s" alt="%s" loading="lazy" decoding="async">`,
				html.EscapeString(src),
				html.EscapeString(alt))
			if alt != "" {
				fmt.Fprintf(&b, `<figcaption>%s</figcaption>`, html.EscapeString(alt))
			}
			b.WriteString(`</figure>`)
			continue
		}
		if level, text, ok := parseMarkdownHeadingBlock(block); ok {
			fmt.Fprintf(&b, `<h%d>%s</h%d>`, level, safeInlineHTML(text), level)
			continue
		}
		if ordered, items, ok := parseMarkdownListBlock(block); ok {
			if ordered {
				b.WriteString(`<ol>`)
			} else {
				b.WriteString(`<ul>`)
			}
			for _, item := range items {
				fmt.Fprintf(&b, `<li>%s</li>`, safeInlineHTML(item))
			}
			if ordered {
				b.WriteString(`</ol>`)
			} else {
				b.WriteString(`</ul>`)
			}
			continue
		}
		if quote, ok := parseMarkdownQuoteBlock(block); ok {
			fmt.Fprintf(&b, `<blockquote><p>%s</p></blockquote>`, safeInlineHTML(quote))
			continue
		}
		fmt.Fprintf(&b, `<p>%s</p>`, safeInlineHTML(block))
	}
	return b.String()
}

func safeInlineHTML(s string) string {
	var b strings.Builder
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '`' {
			code, end, ok := parseInlineCodeAt(s, i)
			if !ok {
				continue
			}
			b.WriteString(html.EscapeString(s[last:i]))
			fmt.Fprintf(&b, `<code>%s</code>`, html.EscapeString(code))
			i = end - 1
			last = end
			continue
		}
		if s[i] == '[' {
			label, href, end, ok := parseMarkdownLinkAt(s, i)
			if !ok {
				continue
			}
			b.WriteString(html.EscapeString(s[last:i]))
			fmt.Fprintf(&b, `<a href="%s" target="_blank" rel="noreferrer">%s</a>`,
				html.EscapeString(href),
				html.EscapeString(label))
			i = end - 1
			last = end
		}
	}
	b.WriteString(html.EscapeString(s[last:]))
	return b.String()
}

func parseInlineCodeAt(s string, start int) (code string, end int, ok bool) {
	if start >= len(s) || s[start] != '`' {
		return "", 0, false
	}
	closeRel := strings.IndexByte(s[start+1:], '`')
	if closeRel < 0 {
		return "", 0, false
	}
	code = strings.TrimSpace(s[start+1 : start+1+closeRel])
	if code == "" {
		return "", 0, false
	}
	return code, start + 1 + closeRel + 1, true
}

func markdownLinksToText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '[' {
			label, _, end, ok := parseMarkdownLinkAt(s, i)
			if ok {
				b.WriteString(label)
				i = end
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func parseMarkdownLinkAt(s string, start int) (label string, href string, end int, ok bool) {
	if start > 0 && s[start-1] == '!' {
		return "", "", 0, false
	}
	closeLabelRel := strings.Index(s[start+1:], "](")
	if closeLabelRel < 0 {
		return "", "", 0, false
	}
	closeLabel := start + 1 + closeLabelRel
	urlStart := closeLabel + 2
	closeURLRel := strings.IndexByte(s[urlStart:], ')')
	if closeURLRel < 0 {
		return "", "", 0, false
	}
	urlEnd := urlStart + closeURLRel
	label = strings.TrimSpace(s[start+1 : closeLabel])
	href = strings.TrimSpace(s[urlStart:urlEnd])
	if label == "" || !isSafePublicURL(href) {
		return "", "", 0, false
	}
	return label, href, urlEnd + 1, true
}

func parseMarkdownImageBlock(block string) (src string, alt string, ok bool) {
	block = strings.TrimSpace(block)
	if !strings.HasPrefix(block, "![") || !strings.HasSuffix(block, ")") {
		return "", "", false
	}
	closeAlt := strings.Index(block, "](")
	if closeAlt < 2 {
		return "", "", false
	}
	alt = strings.TrimSpace(block[2:closeAlt])
	src = strings.TrimSpace(block[closeAlt+2 : len(block)-1])
	if !isSafePublicURL(src) {
		return "", "", false
	}
	return src, alt, true
}

func parseMarkdownCodeBlock(block string) (language string, code string, ok bool) {
	block = strings.Trim(block, "\n")
	lines := strings.Split(block, "\n")
	if len(lines) < 2 {
		return "", "", false
	}

	first := strings.TrimSpace(lines[0])
	last := strings.TrimSpace(lines[len(lines)-1])
	if !strings.HasPrefix(first, "```") || !strings.HasPrefix(last, "```") {
		return "", "", false
	}

	language = sanitizeCodeLanguage(strings.TrimSpace(strings.TrimPrefix(first, "```")))
	code = strings.Join(lines[1:len(lines)-1], "\n")
	code = strings.Trim(code, "\n")
	return language, code, true
}

func parseMarkdownHeadingBlock(block string) (level int, text string, ok bool) {
	block = strings.TrimSpace(block)
	if block == "" || block[0] != '#' {
		return 0, "", false
	}

	for level < len(block) && level < 6 && block[level] == '#' {
		level++
	}
	if level < 2 || level > 5 || level >= len(block) || block[level] != ' ' {
		return 0, "", false
	}

	text = strings.TrimSpace(block[level:])
	if text == "" {
		return 0, "", false
	}
	if level > 4 {
		level = 4
	}
	return level, text, true
}

func parseMarkdownListBlock(block string) (ordered bool, items []string, ok bool) {
	lines := strings.Split(strings.TrimSpace(block), "\n")
	if len(lines) == 0 {
		return false, nil, false
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			if len(items) > 0 && ordered {
				return false, nil, false
			}
			items = append(items, strings.TrimSpace(line[2:]))
			continue
		}
		dot := strings.IndexByte(line, '.')
		if dot > 0 && dot+1 < len(line) && line[dot+1] == ' ' && isAllASCIIDigits(line[:dot]) {
			if len(items) > 0 && !ordered {
				return false, nil, false
			}
			ordered = true
			items = append(items, strings.TrimSpace(line[dot+2:]))
			continue
		}
		return false, nil, false
	}

	return ordered, items, len(items) > 0
}

func parseMarkdownQuoteBlock(block string) (string, bool) {
	lines := strings.Split(strings.TrimSpace(block), "\n")
	if len(lines) == 0 {
		return "", false
	}

	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, ">") {
			return "", false
		}
		parts = append(parts, strings.TrimSpace(strings.TrimPrefix(line, ">")))
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, " "), true
}

func sanitizeCodeLanguage(language string) string {
	language = strings.TrimSpace(strings.ToLower(language))
	if language == "" {
		return ""
	}

	var b strings.Builder
	for _, r := range language {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '+':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isAllASCIIDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isSafePublicURL(rawURL string) bool {
	if strings.ContainsAny(rawURL, "\x00\r\n\t ") {
		return false
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || !parsed.IsAbs() {
		return false
	}
	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func dropLeadingTitleBlocks(blocks []string, titles ...string) []string {
	if len(blocks) == 0 || len(titles) == 0 {
		return blocks
	}
	start := 0
	for start < len(blocks) {
		if _, _, ok := parseMarkdownImageBlock(blocks[start]); ok {
			break
		}
		if !matchesAnyNormalizedTitle(blocks[start], titles) {
			break
		}
		start++
	}
	return blocks[start:]
}

func matchesAnyNormalizedTitle(block string, titles []string) bool {
	block = normalizeComparableText(markdownLinksToText(block))
	if block == "" {
		return false
	}
	for _, title := range titles {
		if block == normalizeComparableText(title) {
			return true
		}
	}
	return false
}

func normalizeComparableText(s string) string {
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	return strings.Trim(s, " \t\n\r.,:;!?—-–\"'«»()[]#>*")
}

func formatDateTime(t time.Time) string {
	if t.IsZero() {
		return "недавно"
	}
	return fmt.Sprintf("%s, %02d:%02d", formatDate(t), t.Hour(), t.Minute())
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

type blogProfile struct {
	Title    string
	Subtitle string
}

func blogSiteProfile(articles []Article, hints FallbackSiteHints) blogProfile {
	titles := []string{
		"На полях",
		"Записано рядом",
		"Тихая лента",
		"Между делом",
		"Поля и страницы",
		"Наблюдения",
	}

	title := titles[blogContentSignature(articles, hints)%len(titles)]
	return blogProfile{
		Title:    title,
		Subtitle: "Добро пожалывать на мой блог",
	}
}

func blogContentSignature(articles []Article, hints FallbackSiteHints) int {
	signature := len(articles) * 17
	for _, r := range hints.Domain {
		signature += int(r)
	}
	for _, keyword := range hints.Keywords {
		for _, r := range keyword {
			signature += int(r)
		}
	}
	for _, article := range articles {
		for _, r := range article.Title {
			signature += int(r)
		}
		signature += article.ID * 31
	}
	if signature < 0 {
		return -signature
	}
	return signature
}

func blogVisibleTopics(articles []Article, hints FallbackSiteHints, limit int) []string {
	if limit <= 0 {
		return nil
	}
	if len(hints.Keywords) > 0 {
		if len(hints.Keywords) < limit {
			limit = len(hints.Keywords)
		}
		return append([]string(nil), hints.Keywords[:limit]...)
	}
	return topCategories(articles, limit)
}

func forumSubtitle(articles []Article, hints FallbackSiteHints) string {
	topics := blogVisibleTopics(articles, hints, 3)
	if len(topics) == 0 {
		return "Разговоры вокруг опубликованных записей, вопросов и коротких заметок из общей ленты."
	}
	return "Разговоры вокруг записей по темам: " + strings.ToLower(strings.Join(topics, ", ")) + "."
}

func blogEntryLabel(article Article) string {
	return "запись в блоге"
}

func articleNotebookLine(article Article) string {
	lines := []string{
		"Эта запись оставлена рядом с другими материалами по теме.",
		"Заметка из тех, к которым удобно возвращаться через пару недель.",
		"Главные тезисы собраны здесь, чтобы не терять их в общей ленте.",
		"Эта тема всплывает в разговорах чаще, чем кажется с первого раза.",
		"Запись оставлена как заметка, без попытки закрыть вопрос навсегда.",
	}
	index := article.ID - 1
	if index < 0 {
		index = 0
	}
	return lines[index%len(lines)]
}

func articleCountLabel(count int) string {
	abs := count
	if abs < 0 {
		abs = -abs
	}

	lastTwo := abs % 100
	if lastTwo >= 11 && lastTwo <= 14 {
		return fmt.Sprintf("%d записей", count)
	}

	switch abs % 10 {
	case 1:
		return fmt.Sprintf("%d запись", count)
	case 2, 3, 4:
		return fmt.Sprintf("%d записи", count)
	default:
		return fmt.Sprintf("%d записей", count)
	}
}

func articleCategory(article Article) string {
	text := strings.ToLower(article.Title + " " + article.Content)

	switch {
	case strings.Contains(text, "tls"), strings.Contains(text, "ssl"), strings.Contains(text, "https"), strings.Contains(text, "сертифик"), strings.Contains(text, "шифр"):
		return "Технологии"
	case strings.Contains(text, "dns"), strings.Contains(text, "bgp"), strings.Contains(text, "tcp"), strings.Contains(text, "udp"), strings.Contains(text, "маршрут"), strings.Contains(text, "routing"), strings.Contains(text, "сеть"), strings.Contains(text, "сетев"):
		return "Практика"
	case strings.Contains(text, "docker"), strings.Contains(text, "kubernetes"), strings.Contains(text, "nginx"), strings.Contains(text, "сервер"), strings.Contains(text, "devops"), strings.Contains(text, "ci/cd"), strings.Contains(text, "инфраструкт"):
		return "Материалы"
	case strings.Contains(text, "postgres"), strings.Contains(text, "mysql"), strings.Contains(text, "sqlite"), strings.Contains(text, "clickhouse"), strings.Contains(text, "sql"), strings.Contains(text, "база данных"):
		return "Архив"
	case strings.Contains(text, "golang"), strings.Contains(text, "go "), strings.Contains(text, "python"), strings.Contains(text, "rust"), strings.Contains(text, "код"), strings.Contains(text, "разработ"):
		return "Инструменты"
	case strings.Contains(text, "self-host"), strings.Contains(text, "самохост"), strings.Contains(text, "homelab"), strings.Contains(text, "домашн"), strings.Contains(text, "nas"):
		return "Опыт"
	default:
		return "Заметки"
	}
}

func contentBlocks(s string) []string {
	s = strings.TrimSpace(normalizeContentBreaks(s))
	if s == "" {
		return nil
	}

	rawBlocks := splitContentBlocks(s)
	blocks := make([]string, 0, len(rawBlocks))
	for _, rawBlock := range rawBlocks {
		rawBlock = strings.Trim(rawBlock, "\n")
		if strings.TrimSpace(rawBlock) == "" {
			continue
		}
		if isTrailingArticleMetadataBlock(plainMarkdownBlockText(rawBlock)) {
			break
		}
		if _, _, ok := parseMarkdownCodeBlock(rawBlock); ok {
			blocks = append(blocks, rawBlock)
			continue
		}
		if _, _, ok := parseMarkdownImageBlock(rawBlock); ok {
			blocks = append(blocks, strings.TrimSpace(rawBlock))
			continue
		}
		if _, _, ok := parseMarkdownHeadingBlock(rawBlock); ok {
			blocks = append(blocks, normalizeMarkdownLineBlock(rawBlock))
			continue
		}
		if _, _, ok := parseMarkdownListBlock(rawBlock); ok {
			blocks = append(blocks, normalizeMarkdownLineBlock(rawBlock))
			continue
		}
		if _, ok := parseMarkdownQuoteBlock(rawBlock); ok {
			blocks = append(blocks, normalizeMarkdownLineBlock(rawBlock))
			continue
		}

		lines := strings.Split(rawBlock, "\n")
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
		blocks = append(blocks, strings.Join(cleanedLines, " "))
	}
	return blocks
}

func splitContentBlocks(s string) []string {
	lines := strings.Split(s, "\n")
	blocks := make([]string, 0, 16)
	current := make([]string, 0, 8)
	inCode := false

	flush := func() {
		if len(current) == 0 {
			return
		}
		block := strings.Trim(strings.Join(current, "\n"), "\n")
		if strings.TrimSpace(block) != "" {
			blocks = append(blocks, block)
		}
		current = current[:0]
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				current = append(current, trimmed)
				flush()
				inCode = false
			} else {
				flush()
				current = append(current, trimmed)
				inCode = true
			}
			continue
		}
		if inCode {
			current = append(current, line)
			continue
		}
		if trimmed == "" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return blocks
}

func normalizeMarkdownLineBlock(block string) string {
	lines := strings.Split(block, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func contentParagraphs(s string) []string {
	blocks := contentBlocks(s)
	if len(blocks) == 0 {
		return nil
	}

	paragraphs := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if _, _, ok := parseMarkdownImageBlock(block); ok {
			continue
		}
		if _, _, ok := parseMarkdownCodeBlock(block); ok {
			continue
		}
		block = plainMarkdownBlockText(block)
		if block != "" {
			paragraphs = append(paragraphs, block)
		}
	}
	return paragraphs
}

func plainMarkdownBlockText(block string) string {
	if _, _, ok := parseMarkdownCodeBlock(block); ok {
		return ""
	}
	if _, _, ok := parseMarkdownImageBlock(block); ok {
		return ""
	}
	if _, text, ok := parseMarkdownHeadingBlock(block); ok {
		return markdownLinksToText(text)
	}
	if _, items, ok := parseMarkdownListBlock(block); ok {
		return markdownLinksToText(strings.Join(items, " "))
	}
	if quote, ok := parseMarkdownQuoteBlock(block); ok {
		return markdownLinksToText(quote)
	}
	block = strings.ReplaceAll(block, "`", "")
	return markdownLinksToText(block)
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
	inCode := false
	for _, line := range strings.Split(s, "\n") {
		raw := strings.TrimRight(line, " \t")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			out = append(out, trimmed)
			inCode = !inCode
			prevBlank = false
			continue
		}
		if inCode {
			out = append(out, raw)
			continue
		}
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

func blogSectionChipsHTML(articles []Article, hints FallbackSiteHints, limit int) string {
	categories := blogVisibleTopics(articles, hints, limit)
	if len(categories) == 0 {
		categories = []string{"Заметки", "Практика", "Материалы"}
	}

	var b strings.Builder
	for _, category := range categories {
		fmt.Fprintf(&b, `<span>%s</span>`, html.EscapeString(category))
	}
	return b.String()
}

func blogSidebarHTML(articles []Article, hints FallbackSiteHints) string {
	var b strings.Builder
	if len(articles) > 0 {
		b.WriteString(`<section class="side-card">
				<h3>Недавно</h3>
				<ul class="mini-list">`)
		for index, article := range articles {
			if index >= 4 {
				break
			}
			fmt.Fprintf(&b, `<li><a href="/article/%d">%s</a><span>%s</span></li>`,
				article.ID,
				html.EscapeString(article.Title),
				html.EscapeString(formatDate(article.CreatedAt)))
		}
		b.WriteString(`</ul>
			</section>`)
	}

	return b.String()
}

func forumSidebarHTML(articles []Article, hints FallbackSiteHints) string {
	var b strings.Builder
	b.WriteString(`<section class="side-card">
			<h3>Рубрики</h3>
			<div class="chips">`)
	b.WriteString(blogSectionChipsHTML(articles, hints, 6))
	b.WriteString(`</div>
		</section>`)

	if len(articles) > 0 {
		b.WriteString(`<section class="side-card">
				<h3>Последние темы</h3>
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
			<h3>Аккаунт</h3>
			<p>Чтобы отвечать в темах и сохранять подписки, войдите в аккаунт или зарегистрируйтесь.</p>
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

func forumLastActivity(article Article) time.Time {
	if article.CreatedAt.IsZero() {
		return time.Time{}
	}
	offsetMinutes := 35 + (article.ID*17)%180
	return article.CreatedAt.Add(time.Duration(offsetMinutes) * time.Minute)
}
