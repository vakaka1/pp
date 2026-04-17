package ppfallback

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"golang.org/x/net/html"
)

const (
	lazyFetchTimeout        = 12 * time.Second
	defaultPublishInterval  = time.Hour
	defaultPublishBatchSize = 3
	contentPublisherTimeout = 45 * time.Second
	defaultArticleFetchUA   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36"
)

type RSS struct {
	Channel Channel `xml:"channel"`
}

type Channel struct {
	Title string `xml:"title"`
	Items []Item `xml:"item"`
}

type Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

type ContentLoader struct {
	db               *FallbackDB
	sources          []string
	log              *zap.Logger
	client           *http.Client
	publishInterval  time.Duration
	publishBatchSize int

	mu           sync.Mutex
	activitySeq  atomic.Uint64
	activityCh   chan struct{}
	processedSeq atomic.Uint64
	fetchFeed    func(context.Context, string) ([]Item, error)
	fetchArticle func(context.Context, string) (string, error)
}

func NewContentLoader(db *FallbackDB, keywords []string, intervalMinutes int, batchSize int, log *zap.Logger) *ContentLoader {
	if log == nil {
		log = zap.NewNop()
	}

	loader := &ContentLoader{
		db:      db,
		sources: buildKeywordRSSSources(keywords),
		log:     log,
		client: &http.Client{
			Timeout: lazyFetchTimeout,
		},
		publishInterval:  resolvePublishInterval(intervalMinutes),
		publishBatchSize: resolvePublishBatchSize(batchSize),
		activityCh:       make(chan struct{}, 1),
	}
	loader.fetchFeed = loader.fetchRSSFeed
	loader.fetchArticle = loader.fetchArticleBody
	return loader
}

func buildKeywordRSSSources(keywords []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(keywords))

	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}

		source := "https://habr.com/ru/rss/search/?q=" + url.QueryEscape(keyword) + "&hl=ru&fl=ru"
		if _, ok := seen[source]; ok {
			continue
		}
		seen[source] = struct{}{}
		out = append(out, source)
	}

	return out
}

func (l *ContentLoader) Run(ctx context.Context) {
	if len(l.sources) == 0 {
		l.log.Info("fallback content publisher disabled: no scraper keywords configured")
		return
	}

	var timer *time.Timer
	var timerCh <-chan time.Time

	resetTimer := func() {
		if timer == nil {
			timer = time.NewTimer(l.publishInterval)
		} else {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(l.publishInterval)
		}
		timerCh = timer.C
	}

	stopTimer := func() {
		if timer != nil && !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timerCh = nil
	}
	defer stopTimer()

	for {
		select {
		case <-ctx.Done():
			return
		case <-l.activityCh:
			if l.activitySeq.Load() != l.processedSeq.Load() && timerCh == nil {
				resetTimer()
			}
		case <-timerCh:
			l.publishCycle(ctx, "proxy-activity")
			if l.activitySeq.Load() != l.processedSeq.Load() {
				resetTimer()
				continue
			}
			stopTimer()
		}
	}
}

func (l *ContentLoader) MarkProxyActivity() {
	l.activitySeq.Add(1)
	select {
	case l.activityCh <- struct{}{}:
	default:
	}
}

func (l *ContentLoader) PublishBatch(ctx context.Context, limit int) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.sources) == 0 {
		return 0, nil
	}
	if limit <= 0 {
		limit = l.publishBatchSize
	}

	published := 0
	for _, source := range l.sources {
		items, err := l.fetchFeed(ctx, source)
		if err != nil {
			l.log.Debug("failed to load keyword RSS feed", zap.String("source", source), zap.Error(err))
			continue
		}

		for _, item := range items {
			if published >= limit {
				return published, nil
			}

			title := strings.TrimSpace(item.Title)
			link := strings.TrimSpace(item.Link)
			if title == "" || link == "" {
				continue
			}

			content, err := l.fetchArticle(ctx, link)
			if err != nil {
				l.log.Debug("failed to fetch article body", zap.String("link", link), zap.Error(err))
				continue
			}

			added, err := l.db.InsertArticle(title, content, link, parsePubDate(item.PubDate))
			if err != nil {
				return published, err
			}
			if !added {
				continue
			}

			published++
			l.log.Info("published keyword article", zap.String("source", source), zap.String("title", title), zap.String("link", link))
		}
	}

	return published, nil
}

func (l *ContentLoader) fetchRSSFeed(ctx context.Context, source string) ([]Item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", defaultArticleFetchUA)
	req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.5")

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected RSS status: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var rss RSS
	if err := xml.Unmarshal(body, &rss); err != nil {
		return nil, err
	}

	return rss.Channel.Items, nil
}

func (l *ContentLoader) fetchArticleBody(ctx context.Context, articleURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, articleURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", defaultArticleFetchUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.7,en;q=0.5")

	resp, err := l.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected article status: %s", resp.Status)
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return "", err
	}

	body := extractArticleTextFromHTML(doc)
	if body == "" {
		return "", fmt.Errorf("article body is empty after parsing")
	}

	return body, nil
}

func (l *ContentLoader) publishCycle(parent context.Context, reason string) {
	currentSeq := l.activitySeq.Load()
	if currentSeq == 0 || currentSeq == l.processedSeq.Load() {
		return
	}

	ctx, cancel := context.WithTimeout(parent, contentPublisherTimeout)
	defer cancel()

	published, err := l.PublishBatch(ctx, l.publishBatchSize)
	if err != nil {
		l.log.Warn("fallback content publisher failed", zap.String("reason", reason), zap.Error(err))
		return
	}

	l.processedSeq.Store(currentSeq)

	if published > 0 {
		l.log.Info("fallback content publisher stored new articles", zap.String("reason", reason), zap.Int("count", published))
		return
	}

	l.log.Debug("fallback content publisher found no new articles after proxy activity", zap.String("reason", reason))
}

func resolvePublishInterval(intervalMinutes int) time.Duration {
	if intervalMinutes <= 0 {
		return defaultPublishInterval
	}
	return time.Duration(intervalMinutes) * time.Minute
}

func resolvePublishBatchSize(batchSize int) int {
	if batchSize <= 0 {
		return defaultPublishBatchSize
	}
	return batchSize
}

func parsePubDate(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Now()
	}

	if ts, err := time.Parse(time.RFC1123Z, raw); err == nil {
		return ts
	}
	if ts, err := time.Parse(time.RFC1123, raw); err == nil {
		return ts
	}
	return time.Now()
}
