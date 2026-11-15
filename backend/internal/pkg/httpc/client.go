package httpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type dialFunc func(ctx context.Context, network, addr string) (net.Conn, error)

type clientPool struct {
	mu      sync.RWMutex
	clients map[string]*http.Client
}

var defaultClientPool = &clientPool{clients: make(map[string]*http.Client)}

type policyRoundTripper struct {
	base         http.RoundTripper
	proxyEnabled bool
	allowPrivate bool
	resolver     ipResolver
}

func (rt *policyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req != nil && req.URL != nil && !rt.allowPrivate {
		// Direct dials enforce DNS + IP validation inside DialContext to pin
		// the resolved IP, while proxy mode validates target URLs here via local DNS
		// because DialContext only sees the configured proxy endpoint.
		if rt.proxyEnabled {
			if err := validateRequestURLWithResolver(req.Context(), rt.resolver, req.URL.String(), false); err != nil {
				return nil, err
			}
		} else if _, err := validateRequestSyntax(req.URL.String()); err != nil {
			return nil, err
		}
	}

	return rt.base.RoundTrip(req)
}

// NewClient creates HTTP client with specified timeout and optional proxy.
// Clients are reused by (timeout, proxy, allowPrivate) to keep connections warm.
func NewClient(timeout time.Duration, proxyURL string, allowPrivate bool) (*http.Client, error) {
	trimmedProxy := strings.TrimSpace(proxyURL)
	proxy, err := ValidateProxyURL(trimmedProxy)
	if err != nil {
		return nil, err
	}

	key := trimmedProxy + "|" + strconv.FormatInt(timeout.Milliseconds(), 10) + "|" + strconv.FormatBool(allowPrivate)

	defaultClientPool.mu.RLock()
	if client, ok := defaultClientPool.clients[key]; ok {
		defaultClientPool.mu.RUnlock()
		return client, nil
	}
	defaultClientPool.mu.RUnlock()

	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	client, transport := newClientWithDeps(timeout, proxy, allowPrivate, net.DefaultResolver, dialer.DialContext)

	defaultClientPool.mu.Lock()
	if existing, ok := defaultClientPool.clients[key]; ok {
		defaultClientPool.mu.Unlock()
		transport.CloseIdleConnections()
		return existing, nil
	}
	defaultClientPool.clients[key] = client
	defaultClientPool.mu.Unlock()

	return client, nil
}

func newClientWithDeps(
	timeout time.Duration,
	proxy *url.URL,
	allowPrivate bool,
	resolver ipResolver,
	dialFn dialFunc,
) (*http.Client, *http.Transport) {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if dialFn == nil {
		dialFn = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}

	transport := &http.Transport{
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          128,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
	}

	if proxy != nil {
		transport.Proxy = http.ProxyURL(proxy)
		transport.DialContext = dialFn
	} else {
		transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			if allowPrivate {
				return dialFn(ctx, network, addr)
			}
			return dialPinnedPublicAddr(ctx, resolver, dialFn, network, addr)
		}
	}

	client := &http.Client{
		Timeout: timeout,
		Transport: &policyRoundTripper{
			base:         transport,
			proxyEnabled: proxy != nil,
			allowPrivate: allowPrivate,
			resolver:     resolver,
		},
		CheckRedirect: redirectValidatorWithResolver(allowPrivate, resolver),
	}

	return client, transport
}

func redirectValidator(allowPrivate bool) func(req *http.Request, via []*http.Request) error {
	return redirectValidatorWithResolver(allowPrivate, net.DefaultResolver)
}

func redirectValidatorWithResolver(allowPrivate bool, resolver ipResolver) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if req == nil || req.URL == nil {
			return nil
		}
		if len(via) >= 10 {
			return errors.New("stopped after 10 redirects")
		}

		return validateRequestURLWithResolver(req.Context(), resolver, req.URL.String(), allowPrivate)
	}
}

func dialPinnedPublicAddr(ctx context.Context, resolver ipResolver, dialFn dialFunc, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	addrs, err := lookupPublicIPs(ctx, resolver, host)
	if err != nil {
		return nil, err
	}

	var lastErr error
	for _, resolved := range addrs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		ip := resolved.IP
		if ip4 := ip.To4(); ip4 != nil {
			ip = ip4
		}

		conn, dialErr := dialFn(ctx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		lastErr = dialErr
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("%w: no addresses", ErrResolveHost)
}

// SetDefaultHeaders adds default headers required for feed fetching.
func SetDefaultHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Rill/1.0; +https://github.com/lightless233/rill)")
}
