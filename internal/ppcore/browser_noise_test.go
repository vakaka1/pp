package ppcore

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	mrand "math/rand"
)

type recordedNoiseRequest struct {
	Method string
	Path   string
	Body   string
}

type fakeNoiseDoer struct {
	requests  []recordedNoiseRequest
	responses map[string]string
}

func (f *fakeNoiseDoer) Do(req *http.Request) (*http.Response, error) {
	body := []byte(nil)
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		_ = req.Body.Close()
	}

	f.requests = append(f.requests, recordedNoiseRequest{
		Method: req.Method,
		Path:   req.URL.Path,
		Body:   string(body),
	})

	key := req.Method + " " + req.URL.Path
	payload := f.responses[key]
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(payload)),
	}, nil
}

func TestExtractBrowserNoiseArticlePaths(t *testing.T) {
	body := `
	<html>
		<body>
			<a href="/article/3">one</a>
			<a href="/article/3">duplicate</a>
			<a href="/thread/9">two</a>
			<a href="/login">ignore</a>
		</body>
	</html>`

	paths := extractBrowserNoiseArticlePaths(body)
	if len(paths) != 2 {
		t.Fatalf("expected 2 unique content paths, got %d: %#v", len(paths), paths)
	}
	if paths[0] != "/article/3" || paths[1] != "/thread/9" {
		t.Fatalf("unexpected extracted paths: %#v", paths)
	}
}

func TestBrowserNoisePreConnectScenarioVisitsLandingArticleAndLogin(t *testing.T) {
	doer := &fakeNoiseDoer{
		responses: map[string]string{
			"GET /":          `<html><body><a href="/article/1">read</a></body></html>`,
			"GET /article/1": `<html><body>article</body></html>`,
			"GET /login":     `<html><body>login</body></html>`,
		},
	}

	sleeps := make([]time.Duration, 0, 3)
	runner := &browserNoiseRunner{
		baseURL: "https://example.com",
		doer:    doer,
		log:     zap.NewNop(),
		rand:    mrand.New(mrand.NewSource(1)),
		sleep: func(delay time.Duration) {
			sleeps = append(sleeps, delay)
		},
		userAgent: browserNoiseUserAgent,
	}

	runner.runPreConnectScenario(context.Background())

	if len(doer.requests) != 3 {
		t.Fatalf("expected 3 browser-noise requests, got %d: %#v", len(doer.requests), doer.requests)
	}
	if doer.requests[0].Method != http.MethodGet || doer.requests[0].Path != "/" {
		t.Fatalf("unexpected first request: %#v", doer.requests[0])
	}
	if doer.requests[1].Method != http.MethodGet || doer.requests[1].Path != "/article/1" {
		t.Fatalf("unexpected second request: %#v", doer.requests[1])
	}
	if doer.requests[2].Method != http.MethodGet || doer.requests[2].Path != "/login" {
		t.Fatalf("unexpected third request: %#v", doer.requests[2])
	}
	if len(sleeps) == 0 {
		t.Fatalf("expected think-time pauses to be recorded")
	}
}

func TestBrowserNoisePresenceDoesNotVisitLogin(t *testing.T) {
	doer := &fakeNoiseDoer{
		responses: map[string]string{
			"GET /":          `<html><body><a href="/article/1">read</a></body></html>`,
			"GET /article/1": `<html><body>article</body></html>`,
			"GET /login":     `<html><body>login</body></html>`,
		},
	}
	runner := &browserNoiseRunner{
		baseURL:   "https://example.com",
		doer:      doer,
		log:       zap.NewNop(),
		rand:      mrand.New(mrand.NewSource(2)),
		sleep:     func(time.Duration) {},
		userAgent: browserNoiseUserAgent,
	}

	for i := 0; i < 20; i++ {
		runner.runPresenceBurst(context.Background())
	}

	for _, request := range doer.requests {
		if request.Path == "/login" {
			t.Fatalf("presence noise must not revisit login after tunnel creation, got requests: %#v", doer.requests)
		}
	}
}
