package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type Fetcher struct {
	client     *http.Client
	userAgent  string
	maxRetries int

	mu      sync.Mutex
	limits  map[string]*hostLimiter
	hostCfg HostConfig
}

type HostConfig struct {
	InitialConcurrency int
	MinConcurrency     int
	MaxConcurrency     int
	GrowAfterSuccess   int
	MinInterval        time.Duration
}

type Option func(*Fetcher)

func WithMaxRetries(n int) Option        { return func(f *Fetcher) { f.maxRetries = n } }
func WithUserAgent(ua string) Option     { return func(f *Fetcher) { f.userAgent = ua } }
func WithTimeout(t time.Duration) Option { return func(f *Fetcher) { f.client.Timeout = t } }
func WithHostConfig(c HostConfig) Option { return func(f *Fetcher) { f.hostCfg = c } }

func defaultHostConfig() HostConfig {
	return HostConfig{
		InitialConcurrency: 4,
		MinConcurrency:     1,
		MaxConcurrency:     16,
		GrowAfterSuccess:   20,
		MinInterval:        50 * time.Millisecond,
	}
}

func New(opts ...Option) *Fetcher {
	jar, _ := cookiejar.New(nil)
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   32,
		MaxConnsPerHost:       0,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
	}
	f := &Fetcher{
		client: &http.Client{
			Transport: transport,
			Jar:       jar,
			Timeout:   30 * time.Second,
		},
		userAgent:  "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36",
		maxRetries: 3,
		limits:     map[string]*hostLimiter{},
		hostCfg:    defaultHostConfig(),
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

func (f *Fetcher) Get(ctx context.Context, target string, headers map[string]string) (*http.Response, error) {
	return f.do(ctx, http.MethodGet, target, nil, "", headers)
}

func (f *Fetcher) PostJSON(ctx context.Context, target string, body any, headers map[string]string) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}
	return f.do(ctx, http.MethodPost, target, data, "application/json", headers)
}

func (f *Fetcher) PostForm(ctx context.Context, target string, values url.Values, headers map[string]string) (*http.Response, error) {
	return f.do(ctx, http.MethodPost, target, []byte(values.Encode()), "application/x-www-form-urlencoded", headers)
}

func (f *Fetcher) GetJSON(ctx context.Context, target string, headers map[string]string, out any) error {
	resp, err := f.Get(ctx, target, headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("GET %s: status %d: %s", target, resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (f *Fetcher) PostJSONInto(ctx context.Context, target string, body any, headers map[string]string, out any) error {
	resp, err := f.PostJSON(ctx, target, body, headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("POST %s: status %d: %s", target, resp.StatusCode, string(raw))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (f *Fetcher) GetBody(ctx context.Context, target string, headers map[string]string, maxBytes int64) ([]byte, error) {
	resp, err := f.Get(ctx, target, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s: status %d", target, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBytes))
}

func (f *Fetcher) limiterFor(target string) *hostLimiter {
	host := hostOf(target)
	f.mu.Lock()
	defer f.mu.Unlock()
	if l, ok := f.limits[host]; ok {
		return l
	}
	l := newHostLimiter(host, f.hostCfg)
	f.limits[host] = l
	return l
}

func (f *Fetcher) do(ctx context.Context, method, target string, body []byte, contentType string, headers map[string]string) (*http.Response, error) {
	lim := f.limiterFor(target)
	var lastErr error
	backoff := time.Second
	for attempt := 0; attempt <= f.maxRetries; attempt++ {
		if attempt > 0 {
			jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff + jitter):
			}
			backoff *= 2
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
		}
		if err := lim.acquire(ctx); err != nil {
			return nil, err
		}

		var reqBody io.Reader
		if body != nil {
			reqBody = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, target, reqBody)
		if err != nil {
			lim.release()
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("User-Agent", f.userAgent)
		req.Header.Set("Accept", "application/json, text/html;q=0.9, */*;q=0.5")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		started := time.Now()
		resp, err := f.client.Do(req)
		dur := time.Since(started)
		lim.release()
		if err != nil {
			lastErr = err
			lim.onFailure(0)
			log.Warn().Err(err).Str("method", method).Str("url", target).Dur("took", dur).Int("attempt", attempt+1).Msg("request failed, retrying")
			continue
		}

		log.Debug().Str("method", method).Str("url", target).Int("status", resp.StatusCode).Dur("took", dur).Int("attempt", attempt+1).Int("budget", lim.snapshot()).Msg("http")

		if shouldRetry(resp.StatusCode) {
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
			retryAfter := parseRetryAfter(resp.Header)
			resp.Body.Close()
			lim.onFailure(retryAfter)
			log.Warn().Int("status", resp.StatusCode).Str("url", target).Dur("took", dur).Int("attempt", attempt+1).Dur("retry_after", retryAfter).Int("budget", lim.snapshot()).Msg("retryable status, backing off")
			continue
		}

		lim.onSuccess()
		return resp, nil
	}
	return nil, fmt.Errorf("after %d attempts: %w", f.maxRetries+1, lastErr)
}

func shouldRetry(status int) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	if status >= 500 && status < 600 {
		return true
	}
	return false
}

func parseRetryAfter(h http.Header) time.Duration {
	v := h.Get("Retry-After")
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}

func hostOf(target string) string {
	if u, err := url.Parse(target); err == nil && u.Host != "" {
		return u.Host
	}
	return target
}

type hostLimiter struct {
	host       string
	cfg        HostConfig
	mu         sync.Mutex
	cap        int
	inFlight   int
	successRun int
	cond       *sync.Cond
	cooldown   time.Time
	lastFire   time.Time
}

func newHostLimiter(host string, cfg HostConfig) *hostLimiter {
	l := &hostLimiter{host: host, cfg: cfg, cap: cfg.InitialConcurrency}
	l.cond = sync.NewCond(&l.mu)
	return l
}

func (l *hostLimiter) snapshot() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cap
}

func (l *hostLimiter) acquire(ctx context.Context) error {
	stop := context.AfterFunc(ctx, func() {
		l.mu.Lock()
		l.cond.Broadcast()
		l.mu.Unlock()
	})
	defer stop()

	l.mu.Lock()
	defer l.mu.Unlock()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		wait := time.Until(l.cooldown)
		if wait > 0 {
			l.mu.Unlock()
			select {
			case <-ctx.Done():
				l.mu.Lock()
				return ctx.Err()
			case <-time.After(wait):
			}
			l.mu.Lock()
			continue
		}
		if l.inFlight >= l.cap {
			l.cond.Wait()
			continue
		}
		gap := time.Until(l.lastFire.Add(l.cfg.MinInterval))
		if gap > 0 {
			l.mu.Unlock()
			select {
			case <-ctx.Done():
				l.mu.Lock()
				return ctx.Err()
			case <-time.After(gap):
			}
			l.mu.Lock()
			continue
		}
		l.inFlight++
		l.lastFire = time.Now()
		return nil
	}
}

func (l *hostLimiter) release() {
	l.mu.Lock()
	if l.inFlight > 0 {
		l.inFlight--
	}
	l.cond.Broadcast()
	l.mu.Unlock()
}

func (l *hostLimiter) onSuccess() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.successRun++
	if l.successRun >= l.cfg.GrowAfterSuccess && l.cap < l.cfg.MaxConcurrency {
		l.cap++
		l.successRun = 0
		log.Debug().Str("host", l.host).Int("cap", l.cap).Msg("aimd grow")
	}
}

func (l *hostLimiter) onFailure(retryAfter time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.successRun = 0
	newCap := l.cap / 2
	if newCap < l.cfg.MinConcurrency {
		newCap = l.cfg.MinConcurrency
	}
	if newCap < l.cap {
		l.cap = newCap
		log.Debug().Str("host", l.host).Int("cap", l.cap).Msg("aimd shrink")
	}
	if retryAfter > 0 {
		l.cooldown = time.Now().Add(retryAfter)
	}
}
