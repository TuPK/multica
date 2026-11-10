package hub

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// Resolver resolves host names. net.DefaultResolver satisfies it; tests inject
// stubs to simulate DNS answers, including rebinding.
type Resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// FetcherOptions configures a SafeFetcher. The zero value is safe: private and
// loopback addresses are refused, redirects are capped, and a 30s deadline applies.
type FetcherOptions struct {
	// AllowPrivate disables the address block list. Tests use it so httptest
	// servers on 127.0.0.1 can be reached. Never set it in production code.
	AllowPrivate bool
	// Timeout bounds one Get, including redirects and body reads. Default 30s,
	// kept under the 45s proxy timeout used elsewhere.
	Timeout time.Duration
	// MaxRedirects caps redirects followed. Default 5.
	MaxRedirects int
	// Resolver defaults to net.DefaultResolver.
	Resolver Resolver
}

const (
	defaultFetchTimeout = 30 * time.Second
	defaultMaxRedirects = 5
	fetchUserAgent      = "multica-hub-importer"
	dialTimeout         = 10 * time.Second
)

// blockedPrefixes lists ranges that are never fetched from. Go's IsLoopback,
// IsPrivate, IsLinkLocalUnicast, IsUnspecified and IsMulticast checks run as
// well; these entries cover the rest, including carrier-grade NAT (where cloud
// metadata endpoints such as 100.100.100.200 live), benchmarking and reserved space.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

// IsBlockedIP reports whether an address must not be dialed. IPv4-mapped IPv6
// addresses are judged as their IPv4 form. A zone suffix is ignored.
func IsBlockedIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	addr = addr.Unmap()
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() || addr.IsUnspecified() || addr.IsMulticast() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// SafeFetcher is an HTTP client that refuses to connect to blocked addresses.
//
// The check runs at dial time, on the address that is actually connected to,
// so DNS rebinding and redirects to internal hosts are caught even when the name
// resolved to a public address earlier. Hostnames are resolved once per dial,
// and only allowed addresses are dialed. The proxy environment is ignored, since
// a proxy would move the connection out of this check.
type SafeFetcher struct {
	opts     FetcherOptions
	resolver Resolver
	client   *http.Client
	// exempt, when set, marks addresses that bypass the block list. Tests use it
	// to allow the loopback test server while still refusing other private
	// addresses, for example a redirect target. It is not set in production.
	exempt func(net.IP) bool
}

// NewSafeFetcher returns a fetcher configured by opts.
func NewSafeFetcher(opts FetcherOptions) *SafeFetcher {
	if opts.Timeout <= 0 {
		opts.Timeout = defaultFetchTimeout
	}
	if opts.MaxRedirects <= 0 {
		opts.MaxRedirects = defaultMaxRedirects
	}
	resolver := opts.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	f := &SafeFetcher{opts: opts, resolver: resolver}
	dialer := &net.Dialer{
		Timeout: dialTimeout,
		Control: func(network, address string, _ syscall.RawConn) error {
			return f.checkAddress(address)
		},
	}
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           f.dialContext(dialer),
		TLSHandshakeTimeout:   dialTimeout,
		ResponseHeaderTimeout: dialTimeout,
		ForceAttemptHTTP2:     true,
	}
	f.client = &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= f.opts.MaxRedirects {
				return fmt.Errorf("hub: stopped after %d redirects", f.opts.MaxRedirects)
			}
			return checkScheme(req.URL)
		},
	}
	return f
}

// Get downloads rawURL and returns its body and the final URL after redirects.
// It fails when the body exceeds maxBytes. The size is checked against
// Content-Length first and then while streaming, so nothing above the cap is
// buffered.
func (f *SafeFetcher) Get(ctx context.Context, rawURL string, maxBytes int64) ([]byte, string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrBadURL, err)
	}
	if err := checkScheme(u); err != nil {
		return nil, "", err
	}
	if u.User != nil {
		return nil, "", fmt.Errorf("%w: credentials in URL are not allowed", ErrBadURL)
	}

	ctx, cancel := context.WithTimeout(ctx, f.opts.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrBadURL, err)
	}
	req.Header.Set("User-Agent", fetchUserAgent)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("hub: fetch %s: %w", u.Redacted(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, "", fmt.Errorf("%w: %s", ErrNotFound, resp.Request.URL.Redacted())
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", fmt.Errorf("hub: fetch %s: HTTP %d", resp.Request.URL.Redacted(), resp.StatusCode)
	}
	if resp.ContentLength > maxBytes {
		return nil, "", fmt.Errorf("%w: %s declares %d bytes, limit %d", ErrTooLarge, resp.Request.URL.Redacted(), resp.ContentLength, maxBytes)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("hub: read %s: %w", resp.Request.URL.Redacted(), err)
	}
	if int64(len(body)) > maxBytes {
		return nil, "", fmt.Errorf("%w: %s exceeds %d bytes", ErrTooLarge, resp.Request.URL.Redacted(), maxBytes)
	}
	return body, resp.Request.URL.String(), nil
}

// dialContext resolves the host, drops blocked addresses, and dials the first
// allowed address that accepts the connection. The dialer's Control hook checks
// the address again just before connect.
func (f *SafeFetcher) dialContext(dialer *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("hub: dial %q: %w", addr, err)
		}
		ips, err := f.resolve(ctx, host)
		if err != nil {
			return nil, err
		}
		var allowed []net.IP
		for _, ip := range ips {
			if f.allowed(ip) {
				allowed = append(allowed, ip)
			}
		}
		if len(allowed) == 0 {
			return nil, fmt.Errorf("%w: %s resolves only to blocked addresses", ErrBlockedAddress, host)
		}
		var lastErr error
		for _, ip := range allowed {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
}

func (f *SafeFetcher) resolve(ctx context.Context, host string) ([]net.IP, error) {
	if ip := parseIPLiteral(host); ip != nil {
		return []net.IP{ip}, nil
	}
	addrs, err := f.resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("hub: resolve %s: %w", host, err)
	}
	ips := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("hub: resolve %s: no addresses", host)
	}
	return ips, nil
}

// checkAddress is the Control hook: it sees the final "ip:port" about to be
// connected and refuses blocked ones.
func (f *SafeFetcher) checkAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBlockedAddress, err)
	}
	ip := parseIPLiteral(host)
	if ip == nil || !f.allowed(ip) {
		return fmt.Errorf("%w: %s", ErrBlockedAddress, host)
	}
	return nil
}

func (f *SafeFetcher) allowed(ip net.IP) bool {
	if f.opts.AllowPrivate {
		return true
	}
	if f.exempt != nil && f.exempt(ip) {
		return true
	}
	return !IsBlockedIP(ip)
}

func parseIPLiteral(host string) net.IP {
	host, _, _ = strings.Cut(host, "%")
	return net.ParseIP(host)
}

func checkScheme(u *url.URL) error {
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("%w: scheme %q is not http or https", ErrBadURL, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("%w: missing host", ErrBadURL)
	}
	return nil
}
