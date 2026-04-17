package ppfallback

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

	forbidden := []string{"fallback", "tunnel", "гостевой"}

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

func TestContentLoaderPublishesKeywordArticles(t *testing.T) {
	db, err := InitFallbackDB("")
	if err != nil {
		t.Fatalf("InitFallbackDB() error = %v", err)
	}

	loader := NewContentLoader(db, []string{"golang"}, 60, 2, zap.NewNop())
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
	if !strings.Contains(body, "Обсуждение доступно участникам") {
		t.Fatalf("expected auth gate in response body, got %q", body)
	}
	if !strings.Contains(body, "/login") {
		t.Fatalf("expected login link in response body, got %q", body)
	}
}
