package ppcore

import (
	"context"
	"fmt"
	"io"
	mrand "math/rand"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/user/pp/internal/config"
	"github.com/user/pp/internal/transport"
	"go.uber.org/zap"
	"golang.org/x/net/html"
)

const (
	browserNoiseDialTimeout        = 8 * time.Second
	browserNoiseRequestTimeout     = 12 * time.Second
	browserNoisePreconnectTimeout  = 18 * time.Second
	browserNoiseLoginCoverTimeout  = 10 * time.Second
	browserNoisePresencePauseMin   = 55 * time.Second
	browserNoisePresencePauseMax   = 3 * time.Minute
	browserNoiseThinkMin           = 900 * time.Millisecond
	browserNoiseThinkMax           = 2600 * time.Millisecond
	browserNoiseBackgroundThinkMin = 700 * time.Millisecond
	browserNoiseBackgroundThinkMax = 1900 * time.Millisecond
	browserNoiseUserAgent          = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36"
)

type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type browserNoisePage struct {
	articlePaths []string
}

type browserNoiseRunner struct {
	baseURL   string
	doer      httpDoer
	rand      *mrand.Rand
	sleep     func(time.Duration)
	log       *zap.Logger
	userAgent string
}

func newBrowserNoiseRunner(cfg *config.ClientConfig, log *zap.Logger) *browserNoiseRunner {
	if log == nil {
		log = zap.NewNop()
	}

	jar, _ := cookiejar.New(nil)
	httpTransport := &http.Transport{
		ForceAttemptHTTP2:   false,
		MaxIdleConns:        4,
		MaxIdleConnsPerHost: 1,
		IdleConnTimeout:     30 * time.Second,
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			timeout := browserNoiseDialTimeout
			if deadline, ok := ctx.Deadline(); ok {
				if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
					timeout = remaining
				}
			}
			return transport.DialTLSHTTP1(addr, cfg.Server.Domain, cfg.Server.TLSFingerprint, timeout)
		},
	}

	return &browserNoiseRunner{
		baseURL:   "https://" + cfg.Server.Domain,
		doer:      &http.Client{Transport: httpTransport, Jar: jar, Timeout: browserNoiseRequestTimeout},
		rand:      mrand.New(mrand.NewSource(time.Now().UnixNano())),
		sleep:     time.Sleep,
		log:       log,
		userAgent: browserNoiseUserAgent,
	}
}

func (b *browserNoiseRunner) runPreConnectScenario(ctx context.Context) {
	page, err := b.fetchPage(ctx, http.MethodGet, "/", "", "")
	if err != nil {
		b.log.Debug("browser noise landing page failed", zap.Error(err))
		return
	}

	if !b.pause(ctx, b.randomDuration(browserNoiseThinkMin, browserNoiseThinkMax)) {
		return
	}

	if articlePath := b.pickArticlePath(page.articlePaths); articlePath != "" {
		if _, err := b.fetchPage(ctx, http.MethodGet, articlePath, "", b.baseURL+"/"); err != nil {
			b.log.Debug("browser noise article visit failed", zap.String("path", articlePath), zap.Error(err))
		}
		if !b.pause(ctx, b.randomDuration(browserNoiseThinkMin, browserNoiseThinkMax)) {
			return
		}
	}

	if _, err := b.fetchPage(ctx, http.MethodGet, "/login", "", b.baseURL+"/"); err != nil {
		b.log.Debug("browser noise login page failed", zap.Error(err))
		return
	}

	_ = b.pause(ctx, b.randomDuration(browserNoiseBackgroundThinkMin, browserNoiseBackgroundThinkMax))
}

func (b *browserNoiseRunner) startLoginCover() {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), browserNoiseLoginCoverTimeout)
		defer cancel()
		if err := b.submitLogin(ctx); err != nil {
			b.log.Debug("browser noise login cover failed", zap.Error(err))
		}
	}()
}

func (b *browserNoiseRunner) RunPresenceLoop(ctx context.Context) {
	for {
		if !b.pause(ctx, b.randomDuration(browserNoisePresencePauseMin, browserNoisePresencePauseMax)) {
			return
		}
		b.runPresenceBurst(ctx)
	}
}

func (b *browserNoiseRunner) runPresenceBurst(ctx context.Context) {
	page, err := b.fetchPage(ctx, http.MethodGet, "/", "", "")
	if err != nil {
		b.log.Debug("browser noise background index failed", zap.Error(err))
		return
	}

	if !b.pause(ctx, b.randomDuration(browserNoiseBackgroundThinkMin, browserNoiseBackgroundThinkMax)) {
		return
	}

	if articlePath := b.pickArticlePath(page.articlePaths); articlePath != "" && b.rand.Intn(100) < 80 {
		if _, err := b.fetchPage(ctx, http.MethodGet, articlePath, "", b.baseURL+"/"); err != nil {
			b.log.Debug("browser noise background article failed", zap.String("path", articlePath), zap.Error(err))
		}
	}

	if b.rand.Intn(100) < 25 {
		if !b.pause(ctx, b.randomDuration(400*time.Millisecond, 1200*time.Millisecond)) {
			return
		}
		if _, err := b.fetchPage(ctx, http.MethodGet, "/login", "", b.baseURL+"/"); err != nil {
			b.log.Debug("browser noise background login page failed", zap.Error(err))
		}
	}
}

func (b *browserNoiseRunner) submitLogin(ctx context.Context) error {
	username, password := b.syntheticCredentials()
	form := url.Values{}
	form.Set("username", username)
	form.Set("password", password)

	_, err := b.fetchPage(ctx, http.MethodPost, "/login", form.Encode(), b.baseURL+"/login")
	return err
}

func (b *browserNoiseRunner) fetchPage(ctx context.Context, method, path, body, referer string) (browserNoisePage, error) {
	target := b.baseURL + path
	var payload io.Reader
	if body != "" {
		payload = strings.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, target, payload)
	if err != nil {
		return browserNoisePage{}, err
	}

	req.Header.Set("User-Agent", b.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "ru-RU,ru;q=0.9,en-US;q=0.7,en;q=0.5")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", b.baseURL)
	}

	resp, err := b.doer.Do(req)
	if err != nil {
		return browserNoisePage{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return browserNoisePage{}, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return browserNoisePage{}, err
	}

	return browserNoisePage{
		articlePaths: extractBrowserNoiseArticlePaths(string(data)),
	}, nil
}

func extractBrowserNoiseArticlePaths(body string) []string {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil
	}

	seen := make(map[string]struct{})
	paths := make([]string, 0, 8)
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil {
			return
		}
		if node.Type == html.ElementNode && node.Data == "a" {
			for _, attr := range node.Attr {
				if attr.Key != "href" {
					continue
				}
				href := strings.TrimSpace(attr.Val)
				if !strings.HasPrefix(href, "/article/") && !strings.HasPrefix(href, "/thread/") {
					continue
				}
				if _, ok := seen[href]; ok {
					break
				}
				seen[href] = struct{}{}
				paths = append(paths, href)
				break
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return paths
}

func (b *browserNoiseRunner) pickArticlePath(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	return paths[b.rand.Intn(len(paths))]
}

func (b *browserNoiseRunner) syntheticCredentials() (string, string) {
	names := []string{"alex", "dmitry", "nikita", "roman", "anna", "irina"}
	suffix := 100 + b.rand.Intn(900)
	return fmt.Sprintf("%s%d", names[b.rand.Intn(len(names))], suffix), fmt.Sprintf("p%04dword", 1000+b.rand.Intn(9000))
}

func (b *browserNoiseRunner) randomDuration(min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	return min + time.Duration(b.rand.Int63n(int64(max-min)+1))
}

func (b *browserNoiseRunner) pause(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}

	if b.sleep != nil {
		done := make(chan struct{})
		go func() {
			b.sleep(delay)
			close(done)
		}()

		select {
		case <-ctx.Done():
			return false
		case <-done:
			return true
		}
	}

	timer := time.NewTimer(delay)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
