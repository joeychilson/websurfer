package headless

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// Response represents the rendered page response.
type Response struct {
	URL        string
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// Browser provides headless browser rendering for SPAs.
type Browser struct {
	timeout time.Duration
	cdpURL  string
	logger  *slog.Logger
}

// Option configures the Browser.
type Option func(*Browser)

// WithTimeout sets the render timeout.
func WithTimeout(d time.Duration) Option {
	return func(b *Browser) {
		b.timeout = d
	}
}

// WithCDPURL sets a remote Chrome DevTools Protocol endpoint.
func WithCDPURL(url string) Option {
	return func(b *Browser) {
		b.cdpURL = url
	}
}

// WithLogger sets the logger.
func WithLogger(l *slog.Logger) Option {
	return func(b *Browser) {
		b.logger = l
	}
}

// New creates a new headless Browser.
func New(opts ...Option) *Browser {
	b := &Browser{
		timeout: 30 * time.Second,
		cdpURL:  os.Getenv("CDP_URL"),
		logger:  slog.Default(),
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// Render fetches a URL using a headless browser and returns the rendered HTML.
func (b *Browser) Render(ctx context.Context, url string) (*Response, error) {
	b.logger.Debug("headless render started", "url", url)

	var (
		allocCtx    context.Context
		allocCancel context.CancelFunc
	)
	if b.cdpURL != "" {
		b.logger.Debug("using remote CDP endpoint", "url", b.cdpURL)
		allocCtx, allocCancel = chromedp.NewRemoteAllocator(ctx, b.cdpURL, chromedp.NoModifyURL)
	} else {
		opts := make([]chromedp.ExecAllocatorOption, len(chromedp.DefaultExecAllocatorOptions))
		copy(opts, chromedp.DefaultExecAllocatorOptions[:])
		opts = append(opts,
			chromedp.Flag("headless", true),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-sandbox", true),
			chromedp.Flag("disable-dev-shm-usage", true),
			chromedp.Flag("disable-extensions", true),
			chromedp.Flag("disable-background-networking", true),
			chromedp.Flag("disable-sync", true),
			chromedp.Flag("disable-translate", true),
			chromedp.Flag("mute-audio", true),
			chromedp.Flag("hide-scrollbars", true),
			chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
		)

		allocCtx, allocCancel = chromedp.NewExecAllocator(ctx, opts...)
	}
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx)
	defer taskCancel()

	taskCtx, timeoutCancel := context.WithTimeout(taskCtx, b.timeout)
	defer timeoutCancel()

	var (
		html       string
		statusCode int
		finalURL   string
		headers    http.Header
	)

	state := &pageState{}

	chromedp.ListenTarget(taskCtx, func(ev any) {
		switch e := ev.(type) {
		case *network.EventRequestWillBeSent:
			state.addRequest()
		case *network.EventLoadingFinished, *network.EventLoadingFailed:
			state.removeRequest()
		case *network.EventResponseReceived:
			if e.Type == network.ResourceTypeDocument {
				statusCode = int(e.Response.Status)
				headers = headersFromNetwork(e.Response.Headers)
			}
		case *page.EventLifecycleEvent:
			state.setLifecycle(e.Name)
		}
	})

	err := chromedp.Run(taskCtx,
		network.Enable(),
		page.Enable(),
		page.SetLifecycleEventsEnabled(true),
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return waitForPageReady(ctx, state, b.logger)
		}),
		chromedp.Location(&finalURL),
		chromedp.OuterHTML("html", &html),
	)
	if err != nil {
		return nil, fmt.Errorf("headless render failed: %w", err)
	}

	b.logger.Debug("headless render completed", "url", url, "final_url", finalURL, "body_size", len(html))

	if statusCode == 0 && len(html) > 0 {
		statusCode = 200
	}

	return &Response{
		URL:        finalURL,
		StatusCode: statusCode,
		Headers:    headers,
		Body:       []byte(html),
	}, nil
}

// pageState tracks the loading state of a page.
type pageState struct {
	mu              sync.Mutex
	inflight        int
	lastNetActivity time.Time
	networkIdle     bool
}

// addRequest increments the number of inflight requests and updates the last activity time.
func (s *pageState) addRequest() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inflight++
	s.lastNetActivity = time.Now()
	s.networkIdle = false
}

// removeRequest decrements the number of inflight requests and updates the last activity time.
func (s *pageState) removeRequest() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inflight > 0 {
		s.inflight--
	}
	s.lastNetActivity = time.Now()
}

// setLifecycle updates the lifecycle state of the page.
func (s *pageState) setLifecycle(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "networkIdle" {
		s.networkIdle = true
	}
}

// getState returns the current state of the page.
func (s *pageState) getState() (inflight int, lastActivity time.Time, networkIdle bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inflight, s.lastNetActivity, s.networkIdle
}

// waitForPageReady waits for the page to be fully rendered using multiple signals
func waitForPageReady(ctx context.Context, state *pageState, logger *slog.Logger) error {
	const (
		pollInterval   = 50 * time.Millisecond
		networkIdleFor = 500 * time.Millisecond
		domStableFor   = 500 * time.Millisecond
		maxWait        = 15 * time.Second
		minWait        = 1 * time.Second
	)

	start := time.Now()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var (
		domStableSince time.Time
		lastMutations  int
		currentMut     int
	)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			elapsed := time.Since(start)

			inflight, lastActivity, networkIdle := state.getState()

			var domSnapshot struct {
				ReadyState    string `json:"readyState"`
				MutationCount int    `json:"mutationCount"`
			}
			err := chromedp.Evaluate(`(() => {
  if (!window.__wsMutationObserver) {
    window.__wsMutationCount = 0;
    if (typeof MutationObserver !== "undefined") {
      const target = document.documentElement || document;
      if (target) {
        const obs = new MutationObserver(() => { window.__wsMutationCount++; });
        obs.observe(target, {childList: true, subtree: true, characterData: true});
        window.__wsMutationObserver = obs;
      }
    }
  }
  return {readyState: document.readyState, mutationCount: window.__wsMutationCount || 0};
})()`, &domSnapshot).Do(ctx)
			if err != nil {
				logger.Debug("failed to evaluate DOM snapshot", "error", err)
			}
			currentMut = domSnapshot.MutationCount

			if currentMut != lastMutations {
				lastMutations = currentMut
				domStableSince = time.Now()
			} else if domStableSince.IsZero() {
				domStableSince = time.Now()
			}

			domStable := !domStableSince.IsZero() && time.Since(domStableSince) >= domStableFor

			netIdle := networkIdle || (inflight == 0 && !lastActivity.IsZero() && time.Since(lastActivity) >= networkIdleFor)

			if elapsed >= minWait && domStable && netIdle {
				logger.Debug("page ready",
					"elapsed", elapsed,
					"mutation_count", currentMut,
					"network_idle", networkIdle,
					"inflight", inflight,
				)
				return nil
			}

			if elapsed >= maxWait {
				logger.Debug("page ready (timeout)",
					"elapsed", elapsed,
					"mutation_count", currentMut,
					"network_idle", networkIdle,
					"dom_stable", domStable,
					"inflight", inflight,
				)
				return nil
			}
		}
	}
}

// headersFromNetwork converts network headers to http headers.
func headersFromNetwork(h network.Headers) http.Header {
	if len(h) == 0 {
		return http.Header{"Content-Type": []string{"text/html; charset=utf-8"}}
	}

	headers := make(http.Header, len(h))
	for key, value := range h {
		headers.Set(key, fmt.Sprint(value))
	}

	if headers.Get("Content-Type") == "" {
		headers.Set("Content-Type", "text/html; charset=utf-8")
	}

	return headers
}
