package auth

import (
	"net"
	"net/http"
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

func Middleware(filter *IPFilter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := parseIP(r)
			if !filter.Allowed(ip) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func parseIP(r *http.Request) net.IP {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return net.ParseIP(host)
}
