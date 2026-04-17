package ppfallback

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

func TestExtractArticleTextFromHTMLRemovesImages(t *testing.T) {
	doc, err := html.Parse(strings.NewReader(`
<!doctype html>
<html>
	<body>
		<main>
			<div class="article-formatted-body article-formatted-body_version-2">
				<h1>Заголовок</h1>
				<p>Первый абзац.</p>
				<figure>
					<img src="/cover.png" alt="cover">
					<figcaption>Подпись, которая тоже не должна попасть.</figcaption>
				</figure>
				<p>Второй <strong>абзац</strong> со <a href="#">ссылкой</a>.</p>
			</div>
		</main>
	</body>
</html>`))
	if err != nil {
		t.Fatalf("html.Parse() error = %v", err)
	}

	text := extractArticleTextFromHTML(doc)

	if strings.Contains(text, "cover") || strings.Contains(text, "Подпись") {
		t.Fatalf("expected parser to skip images and captions, got %q", text)
	}
	if !strings.Contains(text, "Первый абзац.") {
		t.Fatalf("expected first paragraph in parsed text, got %q", text)
	}
	if !strings.Contains(text, "Второй абзац со ссылкой.") {
		t.Fatalf("expected second paragraph in parsed text, got %q", text)
	}
}

func TestContentLoaderPublishesOnlyAfterProxyActivity(t *testing.T) {
	db, err := InitFallbackDB("")
	if err != nil {
		t.Fatalf("InitFallbackDB() error = %v", err)
	}

	loader := NewContentLoader(db, []string{"golang"}, 60, 2, nil)
	loader.sources = []string{"stub://habr"}
	loader.fetchFeed = func(ctx context.Context, source string) ([]Item, error) {
		return []Item{
			{
				Title:   "Статья по ключу",
				Link:    "https://habr.com/ru/articles/77/",
				PubDate: "Mon, 02 Jan 2006 15:04:05 +0300",
			},
		}, nil
	}
	loader.fetchArticle = func(ctx context.Context, link string) (string, error) {
		return "Полный текст статьи.", nil
	}

	loader.publishCycle(context.Background(), "test-without-activity")
	if got := db.ArticleCount(); got != 0 {
		t.Fatalf("expected no publications without proxy activity, got %d", got)
	}

	loader.MarkProxyActivity()
	loader.publishCycle(context.Background(), "test-with-activity")
	if got := db.ArticleCount(); got != 1 {
		t.Fatalf("expected publication after proxy activity, got %d", got)
	}
}

func TestFallbackDBPersistsArticlesToDisk(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "fallback.json")

	db, err := InitFallbackDB(dbPath)
	if err != nil {
		t.Fatalf("InitFallbackDB() error = %v", err)
	}

	createdAt := time.Date(2026, time.April, 15, 10, 0, 0, 0, time.UTC)
	added, err := db.InsertArticle("Полная статья", "Большой текст статьи", "https://habr.com/ru/articles/42/", createdAt)
	if err != nil {
		t.Fatalf("InsertArticle() error = %v", err)
	}
	if !added {
		t.Fatalf("expected first insert to add article")
	}

	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected fallback db file to exist: %v", err)
	}

	reloaded, err := InitFallbackDB(dbPath)
	if err != nil {
		t.Fatalf("InitFallbackDB(reload) error = %v", err)
	}

	if got := reloaded.ArticleCount(); got != 1 {
		t.Fatalf("expected 1 persisted article, got %d", got)
	}

	article, err := reloaded.GetArticle(1)
	if err != nil {
		t.Fatalf("GetArticle() error = %v", err)
	}
	if article == nil || article.Content != "Большой текст статьи" {
		t.Fatalf("unexpected persisted article: %#v", article)
	}
}

func TestResolveFallbackDBPathUsesDefaultWhenEmpty(t *testing.T) {
	t.Setenv("PP_FALLBACK_DB_DIR", "")
	path := ResolveFallbackDBPath("", "Main Tag")

	if path == "" {
		t.Fatalf("expected non-empty fallback db path")
	}
	if !strings.Contains(path, "fallback-main-tag.json") {
		t.Fatalf("unexpected default fallback db path: %q", path)
	}
}
