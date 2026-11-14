package httpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type stubResolver struct {
	fn    func(ctx context.Context, host string) ([]net.IPAddr, error)
	calls atomic.Int32
}

func (s *stubResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	s.calls.Add(1)
	return s.fn(ctx, host)
}

func TestValidateRequestURL(t *testing.T) {
	tests := []struct {
		name         string
		rawURL       string
		allowPrivate bool
		wantErr      error
	}{
		{name: "public ipv4", rawURL: "http://93.184.216.34/feed.xml"},
		{name: "public ipv6", rawURL: "https://[2001:4860:4860::8888]/feed.xml"},
		{name: "loopback blocked", rawURL: "http://127.0.0.1/feed.xml", wantErr: ErrPrivateTarget},
		{name: "ipv4 mapped loopback blocked", rawURL: "http://[::ffff:127.0.0.1]/feed.xml", wantErr: ErrPrivateTarget},
		{name: "localhost blocked", rawURL: "http://localhost/feed.xml", wantErr: ErrPrivateTarget},
		{name: "subdomain localhost blocked", rawURL: "http://api.localhost./feed.xml", wantErr: ErrPrivateTarget},
		{name: "private blocked", rawURL: "http://10.0.0.8/feed.xml", wantErr: ErrPrivateTarget},
		{name: "unsupported scheme", rawURL: "file:///tmp/a", wantErr: ErrInvalidURL},
		{name: "malformed url", rawURL: "://bad", wantErr: ErrInvalidURL},
		{name: "allow private", rawURL: "http://127.0.0.1/feed.xml", allowPrivate: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequestURL(context.Background(), tt.rawURL, tt.allowPrivate)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateRequestURL() unexpected error = %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateRequestURL() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateProxyURL(t *testing.T) {
	tests := []struct {
		name    string
		proxy   string
		wantNil bool
		wantErr error
	}{
		{name: "empty", proxy: "   ", wantNil: true},
		{name: "loopback http proxy", proxy: "http://127.0.0.1:8080"},
		{name: "private socks5h proxy", proxy: "socks5h://10.0.0.2:1080"},
		{name: "unsupported scheme", proxy: "file:///etc/passwd", wantErr: ErrInvalidProxy},
		{name: "missing host", proxy: "http:///path", wantErr: ErrInvalidProxy},
		{name: "malformed", proxy: "not-a-proxy", wantErr: ErrInvalidProxy},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateProxyURL(tt.proxy)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ValidateProxyURL() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateProxyURL() unexpected error = %v", err)
			}
			if (got == nil) != tt.wantNil {
				t.Fatalf("ValidateProxyURL() nil = %v, want %v", got == nil, tt.wantNil)
			}
		})
	}

	if _, err := NewClient(5*time.Second, "ftp://127.0.0.1:21", false); !errors.Is(err, ErrInvalidProxy) {
		t.Fatalf("NewClient() with invalid proxy error = %v, want ErrInvalidProxy", err)
	}
}

func TestRedirectValidatorBlocksPrivateTargets(t *testing.T) {
	t.Run("block private target", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1/feed.xml", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}

		if err := redirectValidator(false)(req, nil); !errors.Is(err, ErrPrivateTarget) {
			t.Fatalf("expected ErrPrivateTarget, got %v", err)
		}
	})

	t.Run("allow private target", func(t *testing.T) {
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1/feed.xml", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}

		if err := redirectValidator(true)(req, nil); err != nil {
			t.Fatalf("expected private redirect target to be allowed, got %v", err)
		}
	})
}

func TestDirectDialPinsValidatedIPAndPreventsRebinding(t *testing.T) {
	t.Run("resolves once and dials pinned public ip directly", func(t *testing.T) {
		var seenHost string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seenHost = r.Host
			_, _ = fmt.Fprint(w, "ok")
		}))
		defer server.Close()

		serverURL, err := url.Parse(server.URL)
		if err != nil {
			t.Fatalf("parse server url: %v", err)
		}

		resolver := &stubResolver{
			fn: func(ctx context.Context, host string) ([]net.IPAddr, error) {
				if host != "feed.example.test" {
					t.Fatalf("unexpected lookup host: %q", host)
				}
				return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
			},
		}

		var dialedAddr string
		baseDialer := &net.Dialer{Timeout: 5 * time.Second}
		client, _ := newClientWithDeps(5*time.Second, nil, false, resolver, func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialedAddr = addr
			return baseDialer.DialContext(ctx, network, serverURL.Host)
		})

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://feed.example.test:"+serverURL.Port()+"/feed.xml", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("client.Do() error = %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)

		if string(body) != "ok" {
			t.Fatalf("unexpected response body %q", string(body))
		}
		if calls := resolver.calls.Load(); calls != 1 {
			t.Fatalf("expected 1 DNS resolution in DialContext, got %d", calls)
		}
		wantDial := "93.184.216.34:" + serverURL.Port()
		if dialedAddr != wantDial {
			t.Fatalf("dialed %q, want pinned IP %q", dialedAddr, wantDial)
		}
		if !strings.HasPrefix(seenHost, "feed.example.test:") {
			t.Fatalf("expected original Host header preserved, got %q", seenHost)
		}
	})

	t.Run("rejects rebinding or mixed private address without dialing", func(t *testing.T) {
		resolver := &stubResolver{
			fn: func(ctx context.Context, host string) ([]net.IPAddr, error) {
				return []net.IPAddr{
					{IP: net.ParseIP("93.184.216.34")},
					{IP: net.ParseIP("127.0.0.1")},
				}, nil
			},
		}

		var dialCalls atomic.Int32
		client, _ := newClientWithDeps(5*time.Second, nil, false, resolver, func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialCalls.Add(1)
			return nil, errors.New("should not dial")
		})

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://rebind.example.test/feed.xml", nil)
		_, err := client.Do(req)
		if !errors.Is(err, ErrPrivateTarget) {
			t.Fatalf("expected ErrPrivateTarget, got %v", err)
		}
		if dialCalls.Load() != 0 {
			t.Fatalf("expected 0 dial calls on blocked address, got %d", dialCalls.Load())
		}
	})
}

func TestProxyModeAllowsPrivateProxyWhileEnforcingTargetPolicy(t *testing.T) {
	var proxyHits atomic.Int32
	proxyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHits.Add(1)
		switch r.URL.Path {
		case "/redirect-private":
			http.Redirect(w, r, "http://10.0.0.5/secret.xml", http.StatusFound)
		default:
			_, _ = fmt.Fprint(w, "proxied-ok")
		}
	}))
	defer proxyServer.Close()

	proxyURL, err := ValidateProxyURL(proxyServer.URL)
	if err != nil {
		t.Fatalf("ValidateProxyURL: %v", err)
	}

	resolver := &stubResolver{
		fn: func(ctx context.Context, host string) ([]net.IPAddr, error) {
			switch host {
			case "public.example.test":
				return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
			case "missing.example.test":
				return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
			default:
				return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
			}
		},
	}

	client, _ := newClientWithDeps(5*time.Second, proxyURL, false, resolver, nil)

	t.Run("private proxy reaches public target", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://public.example.test/feed.xml", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("expected request through private proxy to succeed, got %v", err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "proxied-ok" {
			t.Fatalf("unexpected body %q", string(body))
		}
	})

	t.Run("private proxy blocks private target before proxy request", func(t *testing.T) {
		before := proxyHits.Load()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://127.0.0.1/secret.xml", nil)
		_, err := client.Do(req)
		if !errors.Is(err, ErrPrivateTarget) {
			t.Fatalf("expected ErrPrivateTarget, got %v", err)
		}
		if proxyHits.Load() != before {
			t.Fatalf("expected proxy not to be contacted for blocked target")
		}
	})

	t.Run("private proxy blocks redirect to private target", func(t *testing.T) {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://public.example.test/redirect-private", nil)
		_, err := client.Do(req)
		if !errors.Is(err, ErrPrivateTarget) {
			t.Fatalf("expected ErrPrivateTarget on redirect, got %v", err)
		}
	})

	t.Run("private proxy propagates local DNS failure", func(t *testing.T) {
		before := proxyHits.Load()
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://missing.example.test/feed.xml", nil)
		_, err := client.Do(req)
		if !errors.Is(err, ErrResolveHost) {
			t.Fatalf("expected ErrResolveHost, got %v", err)
		}
		if proxyHits.Load() != before {
			t.Fatalf("expected proxy not to be contacted when DNS fails")
		}
	})
}
