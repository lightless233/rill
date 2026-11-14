package httpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

var (
	// ErrInvalidURL indicates malformed input or an unsupported request URL scheme/host.
	ErrInvalidURL = errors.New("invalid url")
	// ErrInvalidProxy indicates a malformed or unsupported proxy URL.
	ErrInvalidProxy = errors.New("invalid proxy url")
	// ErrPrivateTarget indicates the target or redirect resolved to a blocked local/private host.
	ErrPrivateTarget = errors.New("private host is not allowed")
	// ErrResolveHost indicates DNS resolution of a target hostname failed.
	ErrResolveHost = errors.New("resolve host failed")
)

type ipResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

// ValidateRequestURL verifies that rawURL is an http/https URL and, unless allowPrivate
// is true, does not resolve to a local or private network address.
func ValidateRequestURL(ctx context.Context, rawURL string, allowPrivate bool) error {
	return validateRequestURLWithResolver(ctx, net.DefaultResolver, rawURL, allowPrivate)
}

// ValidateProxyURL validates an optional proxy URL (http, https, socks5, socks5h).
// Configured proxies may point to private or loopback hosts without allowing private feed targets.
func ValidateProxyURL(rawProxy string) (*url.URL, error) {
	trimmed := strings.TrimSpace(rawProxy)
	if trimmed == "" {
		return nil, nil
	}

	parsed, err := url.ParseRequestURI(trimmed)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidProxy, err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "socks5", "socks5h":
	default:
		return nil, fmt.Errorf("%w: unsupported proxy scheme: %s", ErrInvalidProxy, parsed.Scheme)
	}

	if strings.TrimSpace(parsed.Hostname()) == "" {
		return nil, fmt.Errorf("%w: proxy host is required", ErrInvalidProxy)
	}

	return parsed, nil
}

func validateRequestURLWithResolver(ctx context.Context, resolver ipResolver, rawURL string, allowPrivate bool) error {
	host, err := validateRequestSyntax(rawURL)
	if err != nil {
		return err
	}

	if !allowPrivate {
		if _, err := lookupPublicIPs(ctx, resolver, host); err != nil {
			return err
		}
	}

	return nil
}

func validateRequestSyntax(rawURL string) (string, error) {
	parsed, err := url.ParseRequestURI(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("%w: unsupported url scheme: %s", ErrInvalidURL, parsed.Scheme)
	}

	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return "", fmt.Errorf("%w: url host is required", ErrInvalidURL)
	}

	return host, nil
}

func validatePublicHost(ctx context.Context, host string) error {
	_, err := lookupPublicIPs(ctx, net.DefaultResolver, host)
	return err
}

func lookupPublicIPs(ctx context.Context, resolver ipResolver, host string) ([]net.IPAddr, error) {
	cleanHost := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	if cleanHost == "" {
		return nil, fmt.Errorf("%w: url host is required", ErrInvalidURL)
	}
	if cleanHost == "localhost" || strings.HasSuffix(cleanHost, ".localhost") {
		return nil, ErrPrivateTarget
	}

	if addr, err := netip.ParseAddr(host); err == nil {
		if addr.Zone() != "" {
			return nil, ErrPrivateTarget
		}
		unmapped := addr.Unmap()
		if isPrivateOrLocalIP(net.IP(unmapped.AsSlice())) {
			return nil, ErrPrivateTarget
		}
		return []net.IPAddr{{IP: net.IP(unmapped.AsSlice())}}, nil
	}
	if strings.Contains(host, "%") {
		return nil, ErrPrivateTarget
	}

	resolveCtx := ctx
	if resolveCtx == nil {
		resolveCtx = context.Background()
	}

	lookupCtx, cancel := context.WithTimeout(resolveCtx, 2*time.Second)
	defer cancel()

	addrs, err := resolver.LookupIPAddr(lookupCtx, host)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrResolveHost, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("%w: no addresses", ErrResolveHost)
	}

	for _, addr := range addrs {
		if addr.Zone != "" || isPrivateOrLocalIP(addr.IP) {
			return nil, ErrPrivateTarget
		}
	}

	return addrs, nil
}

func isPrivateOrLocalIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalMulticast() || ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.IsInterfaceLocalMulticast() || ip.IsUnspecified()
}
