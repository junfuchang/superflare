package fn

import (
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
)

// DynamicURL holds parsed request URL components. Use ParseRequestURLTo and ParseDynamicUrlWith to avoid global state.
type DynamicURL struct {
	Host       string
	Hostname   string
	Href       string
	Origin     string
	Pathname   string
	Port       string
	Protocol   string
	RemoteHost string
}

// RequestURL is the package-level parsed URL (set by ParseRequestURL). Prefer ParseRequestURLTo and passing *DynamicURL for concurrency-safe use.
var RequestURL DynamicURL

var hostPortRe = regexp.MustCompile(`([\w+\.-]+):(\d+)$`)
var readInterfaceAddrs = net.InterfaceAddrs

func getPort(host string, defaultPort string) (hostname string, port string) {
	hostname = host
	port = defaultPort
	portMatch := hostPortRe.FindStringSubmatch(host)
	if portMatch != nil {
		hostname = portMatch[1]
		port = portMatch[2]
	}
	return
}

// ParseRequestURLTo parses r into a DynamicURL without using package-level state. Prefer this over ParseRequestURL when possible.
func ParseRequestURLTo(r *http.Request) DynamicURL {
	scheme := "http:"
	defaultPort := "80"
	if r != nil && r.TLS != nil {
		scheme = "https:"
		defaultPort = "443"
	}
	host := ""
	remoteHost := ""
	if r != nil {
		host = r.Host
		remoteHost = RequestClientHost(r)
	}
	hostname, port := getPort(host, defaultPort)
	pathname := ""
	requestURI := ""
	if r != nil && r.URL != nil {
		pathname = r.URL.Path
		requestURI = r.RequestURI
	}
	return DynamicURL{
		Host:       host,
		Hostname:   hostname,
		Href:       strings.Join([]string{scheme, "//", host, requestURI}, ""),
		Origin:     strings.Join([]string{scheme, "//", host}, ""),
		Pathname:   pathname,
		Port:       port,
		Protocol:   scheme,
		RemoteHost: remoteHost,
	}
}

// ParseRequestURL parses r and updates package-level RequestURL. For new code, prefer ParseRequestURLTo and ParseDynamicUrlWith.
func ParseRequestURL(r *http.Request) {
	RequestURL = ParseRequestURLTo(r)
}

// ParseDynamicUrlWith substitutes URL placeholders using info. Concurrency-safe when info is request-scoped.
func ParseDynamicUrlWith(url string, info *DynamicURL) string {
	if info == nil {
		return url
	}
	result := url
	result = strings.ReplaceAll(result, "{host}", info.Host)
	result = strings.ReplaceAll(result, "{hostname}", info.Hostname)
	result = strings.ReplaceAll(result, "{href}", info.Href)
	result = strings.ReplaceAll(result, "{origin}", info.Origin)
	result = strings.ReplaceAll(result, "{pathname}", info.Pathname)
	result = strings.ReplaceAll(result, "{port}", info.Port)
	result = strings.ReplaceAll(result, "{protocol}", info.Protocol)
	return result
}

func ParseDynamicUrl(url string) string {
	return ParseDynamicUrlWith(url, &RequestURL)
}

func HostLooksLocalNetwork(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return false
	}
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}
	host = strings.Trim(host, "[]")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".lan") ||
		strings.HasSuffix(host, ".home.arpa") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
	}
	return !strings.Contains(host, ".")
}

func RequestLooksLocalNetwork(r *http.Request) bool {
	if r == nil {
		return false
	}
	if HostLooksLocalNetwork(r.Host) {
		return true
	}
	clientHost := RequestClientHost(r)
	clientIP, ok := parseHostAddr(clientHost)
	if !ok {
		return HostLooksLocalNetwork(clientHost)
	}
	clientIP = clientIP.Unmap()
	return addrLooksLocal(clientIP) && !clientIP.IsLoopback()
}

func RequestClientHost(r *http.Request) string {
	if r == nil {
		return ""
	}
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
			for _, part := range strings.Split(value, ",") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				if splitHost, _, err := net.SplitHostPort(part); err == nil {
					return strings.Trim(splitHost, "[]")
				}
				return strings.Trim(part, "[]")
			}
		}
	}
	if strings.TrimSpace(r.RemoteAddr) == "" {
		return ""
	}
	if splitHost, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return strings.Trim(splitHost, "[]")
	}
	return strings.Trim(r.RemoteAddr, "[]")
}

// LocalURLMayShareNetworkWithHost returns false only for obvious LAN mismatches.
// Unknown hostnames stay true so the existing browser-side probe can decide.
func LocalURLMayShareNetworkWithHost(accessHost string, localRawURL string) bool {
	localHost := strings.TrimSpace(localRawURL)
	if u, err := url.Parse(strings.TrimSpace(localRawURL)); err == nil && u.Hostname() != "" {
		localHost = u.Hostname()
	}
	return LocalHostsMayShareNetwork(accessHost, localHost)
}

func LocalURLMayShareNetworkWithRequest(info *DynamicURL, localRawURL string) bool {
	if info == nil {
		return true
	}
	localHost := strings.TrimSpace(localRawURL)
	if u, err := url.Parse(strings.TrimSpace(localRawURL)); err == nil && u.Hostname() != "" {
		localHost = u.Hostname()
	}
	localIP, ok := parseHostAddr(localHost)
	if !ok {
		return true
	}
	localIP = localIP.Unmap()
	if !addrLooksLocal(localIP) {
		return false
	}

	if matches, known := requestClientMayShareNetwork(info.RemoteHost, localIP); known {
		return matches
	}
	if matches, known := hostMayShareNetworkWithIP(info.Hostname, localIP); known {
		return matches
	}

	if requestCanUseLocalInterfaces(info) {
		matches, checked := localInterfaceMayShareNetworkWithIP(localIP)
		if checked {
			return matches
		}
	}

	return true
}

func LocalHostsMayShareNetwork(accessHost string, localHost string) bool {
	accessIP, accessOK := parseHostAddr(accessHost)
	localIP, localOK := parseHostAddr(localHost)
	if !accessOK || !localOK {
		return true
	}

	accessIP = accessIP.Unmap()
	localIP = localIP.Unmap()

	if accessIP.IsLoopback() {
		return true
	}
	if localIP.IsLoopback() {
		return false
	}
	if !addrLooksLocal(localIP) {
		return false
	}
	if !addrLooksLocal(accessIP) {
		return true
	}
	return addrsLikelyShareLocalNetwork(accessIP, localIP)
}

func parseHostAddr(host string) (netip.Addr, bool) {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return netip.Addr{}, false
	}
	if splitHost, _, err := net.SplitHostPort(host); err == nil {
		host = splitHost
	}
	host = strings.Trim(host, "[]")
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr, true
}

func addrLooksLocal(addr netip.Addr) bool {
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast()
}

func ipv4LocalNetworkKey(addr netip.Addr) [3]byte {
	v4 := addr.As4()
	return [3]byte{v4[0], v4[1], v4[2]}
}

func addrsLikelyShareLocalNetwork(accessIP netip.Addr, localIP netip.Addr) bool {
	if accessIP.IsLoopback() || localIP.IsLoopback() {
		return accessIP.IsLoopback() && localIP.IsLoopback()
	}
	if accessIP.Is4() && localIP.Is4() {
		return ipv4LocalNetworkKey(accessIP) == ipv4LocalNetworkKey(localIP)
	}
	if accessIP.Is6() && localIP.Is6() {
		return ipv6LikelySameLocalNetwork(accessIP, localIP)
	}
	return false
}

func ipv6LikelySameLocalNetwork(accessIP netip.Addr, localIP netip.Addr) bool {
	accessBytes := accessIP.As16()
	localBytes := localIP.As16()
	for i := 0; i < 8; i++ {
		if accessBytes[i] != localBytes[i] {
			return false
		}
	}
	return true
}

func requestClientMayShareNetwork(clientHost string, localIP netip.Addr) (bool, bool) {
	clientIP, ok := parseHostAddr(clientHost)
	if !ok {
		return false, false
	}
	clientIP = clientIP.Unmap()
	if !addrLooksLocal(clientIP) || clientIP.IsLoopback() {
		return false, false
	}
	return addrsLikelyShareLocalNetwork(clientIP, localIP), true
}

func hostMayShareNetworkWithIP(host string, localIP netip.Addr) (bool, bool) {
	hostIP, ok := parseHostAddr(host)
	if !ok {
		return false, false
	}
	hostIP = hostIP.Unmap()
	if !addrLooksLocal(hostIP) || hostIP.IsLoopback() {
		return false, false
	}
	return addrsLikelyShareLocalNetwork(hostIP, localIP), true
}

func requestCanUseLocalInterfaces(info *DynamicURL) bool {
	if info == nil {
		return false
	}
	host := strings.TrimSpace(strings.ToLower(info.Hostname))
	return host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".lan") ||
		strings.HasSuffix(host, ".home.arpa") || (host != "" && !strings.Contains(host, "."))
}

func localInterfaceMayShareNetworkWithIP(localIP netip.Addr) (bool, bool) {
	addrs, err := readInterfaceAddrs()
	if err != nil {
		return false, false
	}
	checked := false
	for _, addr := range addrs {
		ip := interfaceAddrIP(addr)
		if !ip.IsValid() {
			continue
		}
		ip = ip.Unmap()
		if !addrLooksLocal(ip) || ip.IsLoopback() {
			continue
		}
		checked = true
		if addrsLikelyShareLocalNetwork(ip, localIP) {
			return true, true
		}
	}
	return false, checked
}

func interfaceAddrIP(addr net.Addr) netip.Addr {
	switch v := addr.(type) {
	case *net.IPNet:
		if parsed, ok := netip.AddrFromSlice(v.IP); ok {
			return parsed
		}
	case *net.IPAddr:
		if parsed, ok := netip.AddrFromSlice(v.IP); ok {
			return parsed
		}
	}
	return netip.Addr{}
}
