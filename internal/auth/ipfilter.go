package auth

import (
	"net"
	"net/http"
	"strings"
)

type IPFilter struct {
	allowed []*net.IPNet
}

func NewIPFilter(cidrs []string) (*IPFilter, error) {
	f := &IPFilter{}
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			ip := net.ParseIP(cidr)
			if ip == nil {
				continue
			}
			network = &net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(len(ip)*8, len(ip)*8),
			}
		}
		f.allowed = append(f.allowed, network)
	}
	return f, nil
}

func (f *IPFilter) Allowed(ip net.IP) bool {
	if len(f.allowed) == 0 {
		return true
	}
	for _, network := range f.allowed {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func (f *IPFilter) isConfigured() bool {
	return f != nil && len(f.allowed) > 0
}

func Middleware(filter *IPFilter, trustedProxies ...*IPFilter) func(http.Handler) http.Handler {
	var trusted *IPFilter
	if len(trustedProxies) > 0 {
		trusted = trustedProxies[0]
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ClientIP(r, trusted)
			if !filter.Allowed(ip) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func parseIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(host)
}

// ClientIP returns the original client address only when the immediate peer
// is in the explicitly configured trusted proxy list. Forwarded headers from
// untrusted peers are ignored to prevent spoofed rate-limit keys.
func ClientIP(r *http.Request, trusted *IPFilter) net.IP {
	peer := parseIP(r)
	if !trusted.isConfigured() || !trusted.Allowed(peer) {
		return peer
	}
	chain := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(chain) - 1; i >= 0; i-- {
		if !trusted.Allowed(peer) {
			return peer
		}
		forwarded := net.ParseIP(strings.TrimSpace(chain[i]))
		if forwarded == nil {
			return peer
		}
		peer = forwarded
	}
	return peer
}

// RequestIsHTTPS reports whether the request was directly received over TLS
// or came from a trusted proxy that marked it as HTTPS.
func RequestIsHTTPS(r *http.Request, trusted *IPFilter) bool {
	if r.TLS != nil {
		return true
	}
	peer := parseIP(r)
	return trusted.isConfigured() && trusted.Allowed(peer) &&
		strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}
