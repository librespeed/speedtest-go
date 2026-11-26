package web

import (
	"net"
	"regexp"
	"strings"
)

// classifyPrivateIP returns a human-readable description if the IP is a
// private or special-purpose address, or an empty string otherwise.
// Mirrors the PHP getLocalOrPrivateIpInfo() function.
func classifyPrivateIP(ip string) string {
	// Strip IPv4-mapped IPv6 prefix if present
	ip = strings.TrimPrefix(ip, "::ffff:")

	switch {
	case ip == "::1":
		return "localhost IPv6 access"
	case isLinkLocalIPv6(ip):
		return "link-local IPv6 access"
	// ULA IPv6 (fc00::/7): fc00:: - fdff:ffff:...
	case isULAIPv6(ip):
		return "ULA IPv6 access"
	case strings.HasPrefix(ip, "127."):
		return "localhost IPv4 access"
	case strings.HasPrefix(ip, "10."):
		return "private IPv4 access"
	case mustCompile(`^172\.(1[6-9]|2\d|3[01])\.`).MatchString(ip):
		return "private IPv4 access"
	case strings.HasPrefix(ip, "192.168"):
		return "private IPv4 access"
	case strings.HasPrefix(ip, "169.254"):
		return "link-local IPv4 access"
	case mustCompile(`^100\.([6-9][0-9]|1[0-2][0-7])\.`).MatchString(ip):
		return "CGNAT IPv4 access"
	}
	return ""
}

// isULAIPv6 checks if an IP is a Unique Local IPv6 Unicast Address (fc00::/7).
func isULAIPv6(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil || ip.To16() == nil {
		return false
	}
	// fc00::/7 means the first 7 bits are 1111110
	// So the first byte & 0xFE must equal 0xFC
	return ip[0]&0xFE == 0xFC
}

// mustCompile is a helper that compiles a regex and panics on error
// (safe to use for static patterns).
func mustCompile(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}
