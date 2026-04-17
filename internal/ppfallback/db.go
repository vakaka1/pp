package ppfallback

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type FallbackDB struct {
	mu       sync.RWMutex
	path     string
	nextID   int
	articles []Article
}

type Article struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Link      string    `json:"link"`
	CreatedAt time.Time `json:"created_at"`
}

type persistedFallbackDB struct {
	NextID   int       `json:"next_id"`
	Articles []Article `json:"articles"`
}

func ResolveFallbackDBPath(dbPath string, tag string) string {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath != "" {
		return dbPath
	}

	fileName := "fallback-content.json"
	if cleanedTag := sanitizeFallbackTag(tag); cleanedTag != "" {
		fileName = "fallback-" + cleanedTag + ".json"
	}

	candidates := make([]string, 0, 4)
	if baseDir := strings.TrimSpace(os.Getenv("PP_FALLBACK_DB_DIR")); baseDir != "" {
		candidates = append(candidates, filepath.Join(baseDir, fileName))
	}
	candidates = append(candidates,
		filepath.Join("/var/lib/pp", fileName),
		filepath.Join("pp-data", fileName),
		filepath.Join(os.TempDir(), fileName),
	)

	for _, candidate := range candidates {
		if err := os.MkdirAll(filepath.Dir(candidate), 0o755); err == nil {
			return candidate
		}
	}

	return filepath.Join("pp-data", fileName)
}

func InitFallbackDB(dbPath string) (*FallbackDB, error) {
	db := &FallbackDB{
		path:     dbPath,
		nextID:   1,
		articles: make([]Article, 0),
	}

	if dbPath == "" {
		return db, nil
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create fallback db directory: %w", err)
	}

	data, err := os.ReadFile(dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			return db, nil
		}
		return nil, fmt.Errorf("failed to read fallback db: %w", err)
	}
	if len(data) == 0 {
		return db, nil
	}

	var persisted persistedFallbackDB
	if err := json.Unmarshal(data, &persisted); err != nil {
		return nil, fmt.Errorf("failed to parse fallback db: %w", err)
	}

	db.articles = append(db.articles, persisted.Articles...)
	db.nextID = persisted.NextID
	if db.nextID <= 0 {
		db.nextID = maxArticleID(db.articles) + 1
	}

	return db, nil
}

func (db *FallbackDB) InsertArticle(title, content, link string, createdAt time.Time) (bool, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	for _, a := range db.articles {
		if a.Link == link {
			return false, nil
		}
	}

	article := Article{
		ID:        db.nextID,
		Title:     title,
		Content:   content,
		Link:      link,
		CreatedAt: createdAt,
	}
	db.nextID++
	db.articles = append(db.articles, article)

	if err := db.saveLocked(); err != nil {
		db.articles = db.articles[:len(db.articles)-1]
		db.nextID--
		return false, err
	}

	return true, nil
}

func (db *FallbackDB) GetRecentArticles(limit int) ([]Article, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if limit <= 0 || len(db.articles) == 0 {
		return nil, nil
	}

	res := append([]Article(nil), db.articles...)
	sort.Slice(res, func(i, j int) bool {
		if res[i].CreatedAt.Equal(res[j].CreatedAt) {
			return res[i].ID > res[j].ID
		}
		return res[i].CreatedAt.After(res[j].CreatedAt)
	})

	if len(res) > limit {
		res = res[:limit]
	}
	return res, nil
}

func (db *FallbackDB) GetArticle(id int) (*Article, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	for _, a := range db.articles {
		if a.ID == id {
			article := a
			return &article, nil
		}
	}
	return nil, nil
}

func (db *FallbackDB) ArticleCount() int {
	db.mu.RLock()
	defer db.mu.RUnlock()

	return len(db.articles)
}

func (db *FallbackDB) saveLocked() error {
	if db.path == "" {
		return nil
	}

	payload := persistedFallbackDB{
		NextID:   db.nextID,
		Articles: append([]Article(nil), db.articles...),
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode fallback db: %w", err)
	}

	tmpPath := db.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write fallback db temp file: %w", err)
	}
	if err := os.Rename(tmpPath, db.path); err != nil {
		return fmt.Errorf("failed to replace fallback db file: %w", err)
	}

	return nil
}

func maxArticleID(articles []Article) int {
	maxID := 0
	for _, article := range articles {
		if article.ID > maxID {
			maxID = article.ID
		}
	}
	return maxID
}

func sanitizeFallbackTag(tag string) string {
	tag = strings.TrimSpace(strings.ToLower(tag))
	if tag == "" {
		return "main"
	}

	var b strings.Builder
	lastDash := false
	for _, r := range tag {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}

	cleaned := strings.Trim(b.String(), "-")
	if cleaned == "" {
		return "main"
	}
	return cleaned
}
