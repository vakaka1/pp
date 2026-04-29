package ppfallback

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vakaka1/pp/internal/config"
	"go.uber.org/zap"
)

func TestNewFallbackHandlerStartsWithoutSeededArticles(t *testing.T) {
	db, err := InitFallbackDB("")
	if err != nil {
		t.Fatalf("InitFallbackDB() error = %v", err)
	}

	if _, err := NewFallbackHandler("blog", "", "", db); err != nil {
		t.Fatalf("NewFallbackHandler() error = %v", err)
	}

	if got := db.ArticleCount(); got != 0 {
		t.Fatalf("expected empty article store, got %d articles", got)
	}
}

func TestGuestIndexDoesNotCreateArticles(t *testing.T) {
	db, err := InitFallbackDB("")
	if err != nil {
		t.Fatalf("InitFallbackDB() error = %v", err)
	}

	handler, err := NewFallbackHandler("blog", "", "", db)
	if err != nil {
		t.Fatalf("NewFallbackHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if got := db.ArticleCount(); got != 0 {
		t.Fatalf("guest visit must not create content, got %d articles", got)
	}
}

func TestPublicPagesDoNotExposeInternalTerminology(t *testing.T) {
	db, err := InitFallbackDB("")
	if err != nil {
		t.Fatalf("InitFallbackDB() error = %v", err)
	}

	if _, err := db.InsertArticle("Пост о сетях", "Разбор инфраструктуры.\n\nБез служебных меток.", "https://example.com/post", time.Now()); err != nil {
		t.Fatalf("InsertArticle() error = %v", err)
	}

	testCases := []struct {
		name     string
		siteType string
		path     string
	}{
		{name: "blog index", siteType: "blog", path: "/"},
		{name: "blog article", siteType: "blog", path: "/article/1"},
		{name: "forum index", siteType: "forum", path: "/"},
		{name: "forum thread", siteType: "forum", path: "/thread/1"},
	}

	forbidden := []string{
		"fallback",
		"tunnel",
		"гостевой",
		"клуб",
		"источник",
		"открыть источник",
		"материал оформлен",
		"парсер",
		"https://example.com/post",
		"илья",
		"меня зовут",
		"инфраструктурой",
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			handler, err := NewFallbackHandler(tc.siteType, "", "invite", db)
			if err != nil {
				t.Fatalf("NewFallbackHandler() error = %v", err)
			}

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", rec.Code)
			}

			body := strings.ToLower(rec.Body.String())
			for _, word := range forbidden {
				if strings.Contains(body, word) {
					t.Fatalf("unexpected internal term %q in %s", word, tc.name)
				}
			}
		})
	}
}

func TestBlogIndexUsesStaticWelcomeCopy(t *testing.T) {
	db, err := InitFallbackDB("")
	if err != nil {
		t.Fatalf("InitFallbackDB() error = %v", err)
	}

	handler, err := NewFallbackHandler("blog", "", "", db, FallbackSiteHints{
		Domain:   "forest.example",
		Keywords: []string{"Жизнь в лесу", "Новые технологии"},
	})
	if err != nil {
		t.Fatalf("NewFallbackHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Добро пожалывать на мой блог") {
		t.Fatalf("expected welcome copy in public blog page, got %q", body)
	}
	for _, forbidden := range []string{"Жизнь в лесу", "Новые технологии", "О журнале", "На столе", "Комментарии"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("unexpected removed sidebar/topic text %q in public blog page, got %q", forbidden, body)
		}
	}
}

func TestBlogArticleRendersImagesLinksAndCommentsBelowArticle(t *testing.T) {
	db, err := InitFallbackDB("")
	if err != nil {
		t.Fatalf("InitFallbackDB() error = %v", err)
	}

	content := "Пост о сетях\n\n## Подзаголовок\n\nПервый абзац со [ссылкой](https://example.org/page) и `inline`.\n\n```go\nfmt.Println(\"ok\")\n```\n\n- пункт списка\n\n![Схема](https://habrastorage.org/webt/a/b/cover.png)\n\nХабы\n\nGo\n\nТеги\n\nbackend"
	if _, err := db.InsertArticle("Пост о сетях", content, "https://example.com/post", time.Now()); err != nil {
		t.Fatalf("InsertArticle() error = %v", err)
	}

	handler, err := NewFallbackHandler("blog", "", "", db)
	if err != nil {
		t.Fatalf("NewFallbackHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/article/1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if strings.Contains(body, `<p>Пост о сетях</p>`) {
		t.Fatalf("article body must not duplicate title paragraph: %q", body)
	}
	for _, want := range []string{
		`href="https://example.org/page"`,
		`<h2>Подзаголовок</h2>`,
		`<code>inline</code>`,
		`<pre><code class="language-go">`,
		`fmt.Println`,
		`<ul><li>пункт списка</li></ul>`,
		`src="https://habrastorage.org/webt/a/b/cover.png"`,
		`class="article-main"`,
		`class="side-card comment-card"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected %q in article body, got %q", want, body)
		}
	}
	if strings.Index(body, `class="side-card comment-card"`) < strings.Index(body, `</article>`) {
		t.Fatalf("comment card must be rendered below article content, got %q", body)
	}
	for _, forbidden := range []string{"Хабы", "Теги", "backend"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("article body must not render Habr metadata %q: %q", forbidden, body)
		}
	}
}

func TestLoginPageDoesNotRenderInviteField(t *testing.T) {
	db, err := InitFallbackDB("")
	if err != nil {
		t.Fatalf("InitFallbackDB() error = %v", err)
	}

	handler, err := NewFallbackHandler("blog", "", "invite", db)
	if err != nil {
		t.Fatalf("NewFallbackHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if strings.Contains(body, `name="invite_code"`) {
		t.Fatalf("login page must not render invite field: %q", body)
	}
}

func TestContentLoaderPublishesKeywordArticles(t *testing.T) {
	db, err := InitFallbackDB("")
	if err != nil {
		t.Fatalf("InitFallbackDB() error = %v", err)
	}

	loader := NewContentLoader(db, config.FallbackSettings{
		ScraperKeywords:        []string{"golang"},
		PublishMinDelayMinutes: 15,
		PublishMaxDelayMinutes: 30,
		PublishBatchSize:       2,
	}, zap.NewNop())
	loader.sources = []string{"stub://habr"}
	loader.fetchFeed = func(ctx context.Context, source string) ([]Item, error) {
		return []Item{
			{
				Title:       "Статья с Habr",
				Link:        "https://habr.com/ru/articles/1/",
				Description: "Краткое описание",
				PubDate:     "Mon, 02 Jan 2006 15:04:05 +0300",
			},
		}, nil
	}
	loader.fetchArticle = func(ctx context.Context, link string) (string, error) {
		return "Полный текст статьи без изображений.\n\nВторой абзац.", nil
	}

	count, err := loader.PublishBatch(context.Background(), 2)
	if err != nil {
		t.Fatalf("PublishBatch() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 published article, got %d", count)
	}

	if got := db.ArticleCount(); got != 1 {
		t.Fatalf("expected 1 published article, got %d", got)
	}

	article, err := db.GetArticle(1)
	if err != nil {
		t.Fatalf("GetArticle() error = %v", err)
	}
	if article == nil || article.Title != "Статья с Habr" {
		t.Fatalf("unexpected article: %#v", article)
	}
	if article.Content != "Полный текст статьи без изображений.\n\nВторой абзац." {
		t.Fatalf("unexpected article content: %q", article.Content)
	}
}

func TestCommentRouteShowsAuthGate(t *testing.T) {
	db, err := InitFallbackDB("")
	if err != nil {
		t.Fatalf("InitFallbackDB() error = %v", err)
	}

	if _, err := db.InsertArticle("Пост", "Контент", "https://example.com/post", time.Now()); err != nil {
		t.Fatalf("InsertArticle() error = %v", err)
	}

	handler, err := NewFallbackHandler("blog", "", "invite", db)
	if err != nil {
		t.Fatalf("NewFallbackHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/article/1/comment", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "Чтобы оставить комментарий, войдите в аккаунт") {
		t.Fatalf("expected auth gate in response body, got %q", body)
	}
	if !strings.Contains(body, "/login") {
		t.Fatalf("expected login link in response body, got %q", body)
	}
}
