package netutil

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/net/http/httpproxy"
)

// ProxyFromCurrentEnvironment resolves proxy settings for each request instead
// of relying on net/http's cached environment snapshot.
func ProxyFromCurrentEnvironment(req *http.Request) (*url.URL, error) {
	if req == nil || req.URL == nil {
		return nil, nil
	}
	return httpproxy.FromEnvironment().ProxyFunc()(req.URL)
}

type ProxyDialAllowList struct {
	values sync.Map
}

func (list *ProxyDialAllowList) Remember(proxyURL *url.URL) {
	if list == nil || proxyURL == nil {
		return
	}
	host := strings.TrimSpace(proxyURL.Hostname())
	if host == "" {
		return
	}
	port := strings.TrimSpace(proxyURL.Port())
	if port == "" {
		port = defaultProxyPort(proxyURL.Scheme)
	}
	list.values.Store(net.JoinHostPort(host, port), struct{}{})
}

func (list *ProxyDialAllowList) Contains(address string) bool {
	if list == nil {
		return false
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	_, ok := list.values.Load(net.JoinHostPort(host, port))
	return ok
}

func defaultProxyPort(scheme string) string {
	if strings.EqualFold(scheme, "https") {
		return "443"
	}
	return "80"
}
