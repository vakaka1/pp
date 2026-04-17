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
			errMessage = "Для регистрации нужен код приглашения. Публичная регистрация отключена."
		} else if h.inviteCode != "" && invite != h.inviteCode {
			errMessage = "Указанный код приглашения не найден. Запросите актуальный код у администратора сообщества."
		} else {
			errMessage = "Проверка кода временно недоступна. Попробуйте позже."
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, authPageHTML("Регистрация", "Создание новых аккаунтов доступно только по приглашениям.", errMessage, true))
}

func (h *FallbackHandler) serveLogin(w http.ResponseWriter, r *http.Request) {
	errMessage := ""
	if r.Method == http.MethodPost {
		_ = r.ParseForm()
		invite := strings.TrimSpace(r.FormValue("invite_code"))
		if invite == "" {
			errMessage = "Вход по паролю отключён. Для доступа нужен код приглашения."
		} else if h.inviteCode != "" && invite != h.inviteCode {
			errMessage = "Код приглашения не принят. Проверьте написание или дождитесь нового приглашения."
		} else {
			errMessage = "Сервис авторизации временно недоступен. Повторите попытку позже."
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, authPageHTML("Вход", "Вход в закрытый раздел выполняется только по приглашениям.", errMessage, false))
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
		:root { color-scheme: light; --bg:#f6f3ee; --panel:#fffdfa; --ink:#1f2328; --muted:#6b7280; --line:#ddd3c5; --accent:#8f3d22; }
		body { margin:0; font-family: Georgia, "Times New Roman", serif; background:linear-gradient(180deg,#f3ede2 0,#f8f6f1 220px,#f6f3ee 100%); color:var(--ink); }
		.wrap { max-width: 920px; margin: 0 auto; padding: 32px 20px 64px; }
		.top { display:flex; justify-content:space-between; align-items:flex-start; gap:16px; margin-bottom:28px; }
		.brand h1 { margin:0; font-size:42px; font-weight:700; }
		.brand p { margin:8px 0 0; color:var(--muted); max-width:540px; }
		.nav a { margin-left:14px; color:var(--accent); text-decoration:none; font-weight:600; }
		.card { background:var(--panel); border:1px solid var(--line); border-radius:18px; padding:24px; box-shadow:0 14px 40px rgba(70,45,20,.06); margin-bottom:22px; }
		h2 { margin:0 0 10px; font-size:30px; }
		.meta { color:var(--muted); font-size:14px; margin-bottom:14px; }
		.lead { color:#2f3640; line-height:1.75; }
		a.more { color:var(--accent); text-decoration:none; font-weight:700; }
		.hero { background:#2d241f; color:#f6efe5; border-radius:22px; padding:28px; margin-bottom:24px; }
		.hero strong { color:#ffcf8a; }
	</style>
</head>
<body>
	<div class="wrap">
		<header class="top">
			<div class="brand">
				<h1>Тихая Сеть</h1>
				<p>Небольшой русский блог об инфраструктуре, сетях, саморазмещении сервисов и спокойной инженерной работе.</p>
			</div>
			<nav class="nav">
				<a href="/login">Войти</a>
				<a href="/register">Регистрация</a>
			</nav>
		</header>
		<section class="hero">
			<strong>Публикации появляются только после защищённых tunnel-подключений.</strong> Обычный гостевой визит ничего не генерирует и не запускает парсер.
		</section>`)

	if len(articles) == 0 {
		b.WriteString(`<section class="card"><h2>Сайт ждёт первую публикацию</h2><p class="lead">Контент появится только после успешного подключения через защищённый клиентский tunnel. Гостевые HTTP-запросы ничего не публикуют.</p></section>`)
	} else {
		for _, a := range articles {
			fmt.Fprintf(&b, `<article class="card">
				<h2><a href="/article/%d" style="color:inherit;text-decoration:none;">%s</a></h2>
				<p class="meta">Опубликовано %s</p>
				<p class="lead">%s</p>
				<a class="more" href="/article/%d">Читать полностью</a>
			</article>`, a.ID, html.EscapeString(a.Title), formatDate(a.CreatedAt), safeSnippet(a.Content, 420), a.ID)
		}
	}

	b.WriteString(`</div></body></html>`)
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
		:root { --bg:#eef1f5; --panel:#ffffff; --ink:#1d2a38; --muted:#738195; --line:#d7dee8; --accent:#0d5aa7; }
		body { margin:0; font-family: "Segoe UI", Tahoma, sans-serif; background:linear-gradient(180deg,#dde7f2 0,#eef1f5 240px); color:var(--ink); }
		.wrap { max-width: 1040px; margin:0 auto; padding:24px 18px 64px; }
		.top { display:flex; justify-content:space-between; align-items:center; margin-bottom:18px; }
		.title h1 { margin:0; font-size:34px; }
		.title p { margin:6px 0 0; color:var(--muted); }
		.top a { margin-left:12px; color:var(--accent); text-decoration:none; font-weight:600; }
		.panel { background:var(--panel); border:1px solid var(--line); border-radius:16px; overflow:hidden; box-shadow:0 10px 30px rgba(25,56,92,.07); }
		.row { display:grid; grid-template-columns: minmax(0,1fr) 130px 150px; gap:20px; padding:18px 22px; border-top:1px solid var(--line); align-items:center; }
		.row:first-of-type { border-top:none; }
		.row.head { background:#f8fbff; font-weight:700; color:#506173; }
		.topic a { color:#17324f; text-decoration:none; font-size:20px; font-weight:700; }
		.topic p { margin:7px 0 0; color:var(--muted); line-height:1.5; }
		.small { color:var(--muted); font-size:14px; }
		.empty { padding:24px 22px; color:var(--muted); line-height:1.7; }
	</style>
</head>
<body>
	<div class="wrap">
		<header class="top">
			<div class="title">
				<h1>Форум «Наблюдатель»</h1>
				<p>Русскоязычное сообщество про сети, безопасность, инфраструктуру и самодельные сервисы.</p>
			</div>
			<nav>
				<a href="/login">Войти</a>
				<a href="/register">Регистрация</a>
			</nav>
	</header>
		<section class="panel">
			<div class="row head">
				<div>Тема</div>
				<div>Ответы</div>
				<div>Последняя публикация</div>
			</div>`)

	if len(articles) == 0 {
		b.WriteString(`<div class="empty">Темы появятся только после успешных tunnel-подключений. Обычный гостевой визит не создаёт контент, а комментарии по-прежнему не публикуются для незнакомцев.</div>`)
	} else {
		for _, a := range articles {
			fmt.Fprintf(&b, `<div class="row">
				<div class="topic">
					<a href="/thread/%d">%s</a>
					<p>%s</p>
				</div>
				<div class="small">0</div>
				<div class="small">%s</div>
			</div>`, a.ID, html.EscapeString(a.Title), safeSnippet(a.Content, 160), formatDate(a.CreatedAt))
		}
	}

	b.WriteString(`</section></div></body></html>`)
	return b.String()
}

func blogArticleHTML(article *Article, commentURL string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<!DOCTYPE html>
<html lang="ru">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>%s</title>
	<style>
		body { margin:0; font-family: Georgia, "Times New Roman", serif; background:#f5f2eb; color:#1f2328; }
		.wrap { max-width:860px; margin:0 auto; padding:28px 20px 64px; }
		.card { background:#fffdfa; border:1px solid #ddd3c5; border-radius:18px; padding:28px; box-shadow:0 14px 40px rgba(70,45,20,.06); }
		a { color:#8f3d22; text-decoration:none; }
		.meta { color:#6b7280; margin-top:6px; }
		.content { line-height:1.8; margin-top:22px; }
		.discussion { margin-top:34px; border-top:1px solid #ebe3d8; padding-top:24px; }
		.notice { margin-top:18px; background:#f7efe4; border:1px solid #e9d8c0; border-radius:14px; padding:16px; }
	</style>
</head>
<body>
	<div class="wrap">
		<p><a href="/">&larr; На главную</a></p>
		<article class="card">
			<h1>%s</h1>
			<p class="meta">Опубликовано %s</p>
			<div class="content">%s</div>
			<section class="discussion">
				<h2>Комментарии</h2>
				<p>Публичная витрина комментарии не публикует. Для ответа нужен вход по приглашению.</p>
				<div class="notice"><a href="%s">Перейти к форме комментария</a></div>
			</section>
		</article>
	</div>
</body>
</html>`, html.EscapeString(article.Title), html.EscapeString(article.Title), formatDate(article.CreatedAt), safeHTML(article.Content), html.EscapeString(commentURL))
	return b.String()
}

func forumThreadHTML(article *Article, commentURL string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<!DOCTYPE html>
<html lang="ru">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>%s</title>
	<style>
		body { margin:0; font-family: "Segoe UI", Tahoma, sans-serif; background:#eef2f7; color:#1d2a38; }
		.wrap { max-width:960px; margin:0 auto; padding:26px 18px 64px; }
		a { color:#0d5aa7; text-decoration:none; }
		.post, .box { background:#fff; border:1px solid #d7dee8; border-radius:16px; padding:22px; box-shadow:0 10px 26px rgba(25,56,92,.06); }
		.post { margin-bottom:18px; }
		.meta { color:#738195; font-size:14px; }
		.body { margin-top:16px; line-height:1.7; }
		.box { margin-top:22px; background:#f8fbff; }
	</style>
</head>
<body>
	<div class="wrap">
		<p><a href="/">&larr; К списку тем</a></p>
		<section class="post">
			<h1>%s</h1>
			<p class="meta">Тема опубликована %s</p>
			<div class="body">%s</div>
		</section>
		<section class="box">
			<strong>Ответы скрыты от гостей.</strong>
			<p>Публичный fallback-сайт не публикует реальные комментарии. Чтобы оставить ответ, нужен действующий код приглашения.</p>
			<p><a href="%s">Открыть форму ответа</a></p>
		</section>
	</div>
</body>
</html>`, html.EscapeString(article.Title), html.EscapeString(article.Title), formatDate(article.CreatedAt), safeHTML(article.Content), html.EscapeString(commentURL))
	return b.String()
}

func commentGateHTML(article *Article, returnURL string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="ru">
<head>
	<meta charset="UTF-8">
	<meta name="viewport" content="width=device-width, initial-scale=1.0">
	<title>Комментарии закрыты</title>
	<style>
		body { margin:0; min-height:100vh; display:grid; place-items:center; background:radial-gradient(circle at top,#e7eef8,#f6f8fb 48%%,#edf1f7 100%%); font-family:"Segoe UI",Tahoma,sans-serif; color:#203041; }
		.card { width:min(560px,calc(100%% - 32px)); background:#fff; border:1px solid #d7dee8; border-radius:18px; padding:28px; box-shadow:0 18px 50px rgba(31,55,84,.10); }
		h1 { margin:0 0 10px; }
		p { color:#607085; line-height:1.6; }
		a { color:#0d5aa7; text-decoration:none; }
		.note { margin-top:16px; padding:14px 16px; background:#fff5e6; border:1px solid #f0d1a1; border-radius:12px; color:#7d4f08; }
	</style>
</head>
<body>
	<div class="card">
		<p><a href="%s">← Вернуться к публикации</a></p>
		<h1>Комментарии закрыты для гостей</h1>
		<p>Публикация ответа к материалу «%s» доступна только после входа по приглашению.</p>
		<div class="note">На публичной витрине комментарии не отображаются и не публикуются без авторизации.</div>
		<p><a href="/login">Открыть форму входа</a></p>
	</div>
</body>
</html>`, html.EscapeString(returnURL), html.EscapeString(article.Title))
}

func authPageHTML(title, subtitle, errMessage string, includeRegisterFields bool) string {
	var extraFields string
	var buttonLabel string
	if includeRegisterFields {
		extraFields = `<input type="text" name="username" placeholder="Псевдоним" required><br>
		<input type="password" name="password" placeholder="Пароль" required><br>`
		buttonLabel = "Отправить заявку"
	} else {
		extraFields = `<input type="text" name="username" placeholder="Логин" required><br>
		<input type="password" name="password" placeholder="Пароль" required><br>`
		buttonLabel = "Проверить приглашение"
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
		body { margin:0; min-height:100vh; display:grid; place-items:center; background:radial-gradient(circle at top,#e7eef8,#f6f8fb 48%%,#edf1f7 100%%); font-family:"Segoe UI",Tahoma,sans-serif; color:#203041; }
		.card { width:min(460px,calc(100%% - 32px)); background:#fff; border:1px solid #d7dee8; border-radius:18px; padding:28px; box-shadow:0 18px 50px rgba(31,55,84,.10); }
		h1 { margin:0 0 10px; }
		p { color:#607085; line-height:1.6; }
		input { width:100%%; box-sizing:border-box; padding:12px 14px; margin:10px 0 0; border-radius:12px; border:1px solid #cfd7e3; }
		button { margin-top:14px; width:100%%; padding:12px 14px; border:none; border-radius:12px; background:#0d5aa7; color:#fff; font-weight:700; cursor:pointer; }
		.note { margin-top:16px; font-size:14px; color:#738195; }
		.msg { margin:16px 0; padding:12px 14px; background:#fff5e6; border:1px solid #f0d1a1; border-radius:12px; color:#7d4f08; }
		a { color:#0d5aa7; text-decoration:none; }
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
			<input type="text" name="invite_code" placeholder="Код приглашения" required><br>
			<button type="submit">%s</button>
		</form>
		<div class="note">Если у вас нет приглашения, дождитесь следующей волны инвайтов. Открытая регистрация не проводится.</div>
	</div>
</body>
</html>`, title, title, subtitle, msg, extraFields, buttonLabel)
}

func formatDate(t time.Time) string {
	months := []string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}
	month := months[int(t.Month())-1]
	return fmt.Sprintf("%d %s %d", t.Day(), month, t.Year())
}

func safeSnippet(s string, limit int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "<br>", " "))
	s = strings.ReplaceAll(s, "\n", " ")
	s = html.EscapeString(s)
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "..."
}

func safeHTML(s string) string {
	escaped := html.EscapeString(strings.TrimSpace(s))
	escaped = strings.ReplaceAll(escaped, "\n", "<br>")
	escaped = strings.ReplaceAll(escaped, "&lt;br&gt;", "<br>")
	return escaped
}
