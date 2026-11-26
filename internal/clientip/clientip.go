// Package clientip resolves the client IP address for an HTTP request.
//
// The TCP peer address is authoritative unless it belongs to a configured
// trusted proxy. Forwarding headers are considered only for trusted peers,
// because an untrusted client can set any of those headers itself. For a
// trusted peer, X-Forwarded-For is read from right to left so that entries
// appended by the client (on the left) cannot override the address appended by
// the proxy.
package clientip

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ParseTrustedProxies parses trusted proxy entries. Each entry is either a
// CIDR prefix or a bare IP address, which is treated as a single-host prefix.
// Empty and whitespace-only entries are ignored.
func ParseTrustedProxies(entries []string) ([]netip.Prefix, error) {
	prefixes := make([]netip.Prefix, 0, len(entries))
	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}

		var prefix netip.Prefix
		var err error
		if strings.Contains(entry, "/") {
			prefix, err = netip.ParsePrefix(entry)
		} else {
			addr, addrErr := netip.ParseAddr(entry)
			if addrErr != nil {
				err = addrErr
			} else {
				addr = normalizeAddr(addr)
				prefix = netip.PrefixFrom(addr, addr.BitLen())
			}
		}
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy %q: %w", entry, err)
		}

		prefixes = append(prefixes, normalizePrefix(prefix))
	}
	return prefixes, nil
}

// ClientIP returns the client IP address for r. It trusts forwarding headers
// only when the TCP peer is covered by trusted. For trusted peers, a valid
// CF-Connecting-IPv6 is used first, then Client-IP, X-Real-IP, and finally
// X-Forwarded-For. If the trusted list cannot be parsed, this function fails
// closed and returns the TCP peer; callers should validate the configuration
// with ParseTrustedProxies at startup.
func ClientIP(r *http.Request, trusted []string) string {
	if r == nil {
		return ""
	}

	peer, ok := parsePeer(r.RemoteAddr)
	if !ok {
		return ""
	}

	if len(trusted) == 0 {
		return peer.String()
	}
	prefixes, err := ParseTrustedProxies(trusted)
	if err != nil || len(prefixes) == 0 {
		return peer.String()
	}
	if !isTrusted(peer, prefixes) {
		return peer.String()
	}

	if ip := singleHeaderIP(r, "CF-Connecting-IPv6", true); ip != "" {
		return ip
	}
	if ip := singleHeaderIP(r, "Client-IP", false); ip != "" {
		return ip
	}
	if ip := singleHeaderIP(r, "X-Real-IP", false); ip != "" {
		return ip
	}
	if ip := forwardedIP(r, prefixes); ip != "" {
		return ip
	}
	return peer.String()
}

// parsePeer extracts and normalizes the address from r.RemoteAddr. The port is
// optional, so both "192.0.2.1:1234" and "192.0.2.1" are accepted.
func parsePeer(remoteAddr string) (netip.Addr, bool) {
	remoteAddr = strings.TrimSpace(remoteAddr)
	if remoteAddr == "" {
		return netip.Addr{}, false
	}

	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	} else if strings.HasPrefix(remoteAddr, "[") && strings.HasSuffix(remoteAddr, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(remoteAddr, "["), "]")
	}

	return parseAddr(host)
}

// singleHeaderIP returns the address from the last occurrence of a header that
// must contain exactly one IP address. Comma-separated values are rejected
// rather than taking the leftmost (client-controlled) token.
func singleHeaderIP(r *http.Request, header string, requireIPv6 bool) string {
	values := r.Header.Values(header)
	if len(values) == 0 {
		return ""
	}
	raw := values[len(values)-1]
	if strings.Contains(raw, ",") {
		return ""
	}

	addr, ok := parseAddr(raw)
	if !ok || (requireIPv6 && !addr.Is6()) {
		return ""
	}
	return addr.String()
}

// forwardedIP walks X-Forwarded-For from right to left and returns the first
// address outside the trusted set. Unparseable entries fail closed: the caller
// falls back to the TCP peer rather than trusting a partial chain.
func forwardedIP(r *http.Request, trusted []netip.Prefix) string {
	var candidates []string
	for _, value := range r.Header.Values("X-Forwarded-For") {
		candidates = append(candidates, strings.Split(value, ",")...)
	}

	for i := len(candidates) - 1; i >= 0; i-- {
		addr, ok := parseAddr(candidates[i])
		if !ok {
			return ""
		}
		if isTrusted(addr, trusted) {
			continue
		}
		return addr.String()
	}
	return ""
}

func isTrusted(addr netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func parseAddr(raw string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil {
		return netip.Addr{}, false
	}
	return normalizeAddr(addr), true
}

func normalizeAddr(addr netip.Addr) netip.Addr {
	if addr.Is4In6() {
		addr = addr.Unmap()
	}
	if addr.Zone() != "" {
		addr = addr.WithZone("")
	}
	return addr
}

func normalizePrefix(prefix netip.Prefix) netip.Prefix {
	prefix = prefix.Masked()
	addr := prefix.Addr()
	if addr.Is4In6() && prefix.Bits() >= 96 {
		return netip.PrefixFrom(addr.Unmap(), prefix.Bits()-96).Masked()
	}
	return prefix
}
