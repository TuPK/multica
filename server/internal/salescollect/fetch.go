package salescollect

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"
)

// UserAgent identifies the collector to the sites it reads. It names the product and
// nothing about the workspace.
const UserAgent = "MulticaSalesCollect/1.0 (+https://multica.ai)"

// robotsToken is the name a robots.txt group is matched against.
const robotsToken = "MulticaSalesCollect"

// Options configure one run or one probe. The zero value is the production configuration.
type Options struct {
	// HTTP overrides the client. When nil, a guarded client is built.
	HTTP *http.Client
	// Timeout bounds each request. Zero means 20 seconds.
	Timeout time.Duration
	// MaxBytes caps each response body. Zero means 2 MiB.
	MaxBytes int64
	// RateInterval is the minimum gap between two requests to one host. Zero means one second.
	RateInterval time.Duration
	// Region and Sector are copied to each signal, from the lane.
	Region string
	Sector string
	// SourceID is copied to each signal, from the source document.
	SourceID string
	// CSVPath is the file a csv_drop recipe reads. It comes from the command line, never a recipe.
	CSVPath string
	// Limit caps the signals a run keeps, below the recipe's max_items. Zero keeps the recipe's cap.
	Limit int

	// allowPrivate lets requests reach private and loopback addresses. Tests set it, through
	// the test-only helper in export_test.go; no production path can.
	allowPrivate bool
}

const (
	defaultTimeout  = 20 * time.Second
	defaultMaxBytes = 2 << 20
	defaultInterval = time.Second
	maxRedirects    = 5
)

var cgnatPrefix = netip.MustParsePrefix("100.64.0.0/10")

// blockedAddr reports whether an address is one the collector never connects to: loopback,
// private, link-local (which includes the cloud metadata address), multicast, unspecified,
// and the carrier-grade NAT range.
func blockedAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || cgnatPrefix.Contains(ip)
}

// guardControl runs on the address a connection is about to use. Checking there, not on the
// name, also stops a host that resolves to a private address after the check (DNS rebinding).
func guardControl(allowPrivate bool) func(network, address string, _ syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
		if allowPrivate {
			return nil
		}
		ap, err := netip.ParseAddrPort(address)
		if err != nil {
			return fmt.Errorf("refusing to connect to %q: %w", address, err)
		}
		if blockedAddr(ap.Addr()) {
			return fmt.Errorf("refusing to connect to %s: a private, loopback or metadata address", ap.Addr())
		}
		return nil
	}
}

// newClient builds the client a run uses. Proxies are off: an environment proxy would carry
// the request to an address the guard never sees.
func newClient(opts Options) *http.Client {
	if opts.HTTP != nil {
		return opts.HTTP
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	dialer := &net.Dialer{Timeout: timeout, Control: guardControl(opts.allowPrivate)}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   timeout * 2,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("refusing redirect to scheme %q", req.URL.Scheme)
			}
			return nil
		},
	}
}

// fetcher performs the requests of one run: it enforces the rate limit per host, checks
// robots.txt once per host, and caps the body size.
type fetcher struct {
	client   *http.Client
	maxBytes int64
	interval time.Duration

	mu      sync.Mutex
	lastHit map[string]time.Time
	robots  map[string]*robotsRules
}

func newFetcher(opts Options) *fetcher {
	maxBytes := opts.MaxBytes
	if maxBytes == 0 {
		maxBytes = defaultMaxBytes
	}
	interval := opts.RateInterval
	if interval == 0 {
		interval = defaultInterval
	}
	return &fetcher{
		client:   newClient(opts),
		maxBytes: maxBytes,
		interval: interval,
		lastHit:  map[string]time.Time{},
		robots:   map[string]*robotsRules{},
	}
}

// Response is a fetched body with its status.
type Response struct {
	Status      int
	ContentType string
	Body        []byte
	FinalURL    string
}

// get fetches a URL after the robots check and the host's rate limit. A disallowed path is
// an error, not a silent empty result.
func (f *fetcher) get(ctx context.Context, rawURL string) (*Response, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("url scheme %q is not http or https", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("url has no host")
	}
	if err := f.allowedByRobots(ctx, u); err != nil {
		return nil, err
	}
	if err := f.pace(ctx, u.Host); err != nil {
		return nil, err
	}
	return f.do(ctx, u.String())
}

func (f *fetcher) do(ctx context.Context, rawURL string) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json, application/rss+xml, application/atom+xml, application/xml, text/html;q=0.9, */*;q=0.5")
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if int64(len(body)) > f.maxBytes {
		return nil, fmt.Errorf("response is larger than %d bytes", f.maxBytes)
	}
	return &Response{
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Body:        body,
		FinalURL:    resp.Request.URL.String(),
	}, nil
}

// pace waits until the host's last request is one interval old.
func (f *fetcher) pace(ctx context.Context, host string) error {
	f.mu.Lock()
	wait := time.Until(f.lastHit[host].Add(f.interval))
	f.lastHit[host] = time.Now().Add(max(wait, 0))
	f.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// allowedByRobots fetches and caches the host's robots.txt, and refuses a path it disallows.
// A robots file that cannot be read stops the host for this run: the collector does not guess.
func (f *fetcher) allowedByRobots(ctx context.Context, u *url.URL) error {
	key := u.Scheme + "://" + u.Host
	f.mu.Lock()
	rules, ok := f.robots[key]
	f.mu.Unlock()
	if !ok {
		resp, err := f.do(ctx, key+"/robots.txt")
		switch {
		case err != nil:
			return fmt.Errorf("robots.txt of %s could not be read: %w", u.Host, err)
		case resp.Status == http.StatusNotFound || resp.Status == http.StatusGone:
			rules = &robotsRules{}
		case resp.Status >= 200 && resp.Status < 300:
			rules = parseRobots(string(resp.Body))
		default:
			return fmt.Errorf("robots.txt of %s answered %d", u.Host, resp.Status)
		}
		f.mu.Lock()
		f.robots[key] = rules
		f.mu.Unlock()
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if !rules.allows(path) {
		return fmt.Errorf("robots.txt of %s disallows %s", u.Host, path)
	}
	return nil
}

// robotsRules holds the Allow and Disallow paths of the group that applies to the collector.
type robotsRules struct {
	allow, disallow []string
}

// robotsGroup is one user-agent group of a robots.txt: the agents it names and its rules.
type robotsGroup struct {
	agents []string
	rules  robotsRules
}

// parseRobots reads the group that names our token, or the * group when none does. A group
// starts at a user-agent line that follows a rule line, as the robots.txt convention has it.
func parseRobots(body string) *robotsRules {
	var groups []*robotsGroup
	var current *robotsGroup
	lastWasAgent := false
	for _, raw := range strings.Split(body, "\n") {
		line := raw
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			if current == nil || !lastWasAgent {
				current = &robotsGroup{}
				groups = append(groups, current)
			}
			current.agents = append(current.agents, strings.ToLower(value))
			lastWasAgent = true
		case "allow", "disallow":
			lastWasAgent = false
			if current == nil {
				continue
			}
			if key == "allow" {
				current.rules.allow = append(current.rules.allow, value)
			} else if value != "" {
				current.rules.disallow = append(current.rules.disallow, value)
			}
		default:
			lastWasAgent = false
		}
	}
	token := strings.ToLower(robotsToken)
	var wildcard *robotsRules
	for _, g := range groups {
		for _, a := range g.agents {
			if a == token {
				return &g.rules
			}
			if a == "*" && wildcard == nil {
				wildcard = &g.rules
			}
		}
	}
	if wildcard != nil {
		return wildcard
	}
	return &robotsRules{}
}

// allows applies the longest matching rule: an Allow wins a tie, and no match allows.
func (r *robotsRules) allows(path string) bool {
	longestDisallow, longestAllow := -1, -1
	for _, p := range r.disallow {
		if strings.HasPrefix(path, p) && len(p) > longestDisallow {
			longestDisallow = len(p)
		}
	}
	for _, p := range r.allow {
		if strings.HasPrefix(path, p) && len(p) > longestAllow {
			longestAllow = len(p)
		}
	}
	if longestDisallow < 0 {
		return true
	}
	return longestAllow >= longestDisallow
}
