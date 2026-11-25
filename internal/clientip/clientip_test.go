package clientip

import (
	"net/http/httptest"
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		name    string
		remote  string
		headers map[string][]string
		trusted []string
		want    string
	}{
		{
			name:   "untrusted peer ignores spoofed headers",
			remote: "198.51.100.10:1234",
			headers: map[string][]string{
				"CF-Connecting-IPv6": {"2001:db8::1"},
				"Client-IP":          {"203.0.113.1"},
				"X-Real-IP":          {"203.0.113.2"},
				"X-Forwarded-For":    {"203.0.113.3"},
			},
			trusted: []string{"10.0.0.0/8"},
			want:    "198.51.100.10",
		},
		{
			name:   "empty trusted list ignores headers",
			remote: "198.51.100.10:1234",
			headers: map[string][]string{
				"X-Real-IP":       {"203.0.113.2"},
				"X-Forwarded-For": {"203.0.113.3"},
			},
			want: "198.51.100.10",
		},
		{
			name:    "trusted peer skips trusted rightmost XFF entry",
			remote:  "10.0.0.5:1234",
			headers: map[string][]string{"X-Forwarded-For": {"1.2.3.4, 10.0.0.5"}},
			trusted: []string{"10.0.0.0/8"},
			want:    "1.2.3.4",
		},
		{
			name:    "trusted peer ignores spoofed leftmost XFF entry",
			remote:  "10.0.0.5:1234",
			headers: map[string][]string{"X-Forwarded-For": {"9.9.9.9, 1.2.3.4"}},
			trusted: []string{"10.0.0.0/8"},
			want:    "1.2.3.4",
		},
		{
			name:   "X-Real-IP wins over X-Forwarded-For",
			remote: "10.0.0.5:1234",
			headers: map[string][]string{
				"X-Real-IP":       {"5.6.7.8"},
				"X-Forwarded-For": {"1.2.3.4"},
			},
			trusted: []string{"10.0.0.0/8"},
			want:    "5.6.7.8",
		},
		{
			name:   "Client-IP wins over X-Real-IP",
			remote: "10.0.0.5:1234",
			headers: map[string][]string{
				"Client-IP":       {"5.6.7.8"},
				"X-Real-IP":       {"9.9.9.9"},
				"X-Forwarded-For": {"1.2.3.4"},
			},
			trusted: []string{"10.0.0.0/8"},
			want:    "5.6.7.8",
		},
		{
			name:   "CF-Connecting-IPv6 wins and accepts IPv6",
			remote: "10.0.0.5:1234",
			headers: map[string][]string{
				"CF-Connecting-IPv6": {"2001:db8::7"},
				"Client-IP":          {"5.6.7.8"},
			},
			trusted: []string{"10.0.0.0/8"},
			want:    "2001:db8::7",
		},
		{
			name:   "CF-Connecting-IPv6 rejects IPv4 values",
			remote: "10.0.0.5:1234",
			headers: map[string][]string{
				"CF-Connecting-IPv6": {"5.6.7.8"},
				"Client-IP":          {"9.9.9.9"},
			},
			trusted: []string{"10.0.0.0/8"},
			want:    "9.9.9.9",
		},
		{
			name:    "unparseable mid-chain XFF fails closed",
			remote:  "10.0.0.5:1234",
			headers: map[string][]string{"X-Forwarded-For": {"1.2.3.4, garbage, 10.0.0.5"}},
			trusted: []string{"10.0.0.0/8"},
			want:    "10.0.0.5",
		},
		{
			name:    "comma-separated single-IP header is rejected",
			remote:  "10.0.0.5:1234",
			headers: map[string][]string{"X-Real-IP": {"1.2.3.4, 5.6.7.8"}},
			trusted: []string{"10.0.0.0/8"},
			want:    "10.0.0.5",
		},
		{
			name:    "last single-IP header value wins",
			remote:  "10.0.0.5:1234",
			headers: map[string][]string{"X-Real-IP": {"1.2.3.4", "5.6.7.8"}},
			trusted: []string{"10.0.0.0/8"},
			want:    "5.6.7.8",
		},
		{
			name:    "right-to-left walk spans multiple XFF header lines",
			remote:  "10.0.0.5:1234",
			headers: map[string][]string{"X-Forwarded-For": {"1.2.3.4, 10.0.0.6", "5.6.7.8"}},
			trusted: []string{"10.0.0.0/8"},
			want:    "5.6.7.8",
		},
		{
			name:    "XFF with only trusted addresses falls back to peer",
			remote:  "10.0.0.5:1234",
			headers: map[string][]string{"X-Forwarded-For": {"10.0.0.6, 10.0.0.7"}},
			trusted: []string{"10.0.0.0/8"},
			want:    "10.0.0.5",
		},
		{
			name:    "IPv4-mapped peer is normalized",
			remote:  "::ffff:192.0.2.7",
			headers: nil,
			want:    "192.0.2.7",
		},
		{
			name:    "bare IPv4 RemoteAddr works",
			remote:  "192.0.2.7",
			headers: nil,
			want:    "192.0.2.7",
		},
		{
			name:    "bare IPv6 RemoteAddr works",
			remote:  "2001:db8::1",
			headers: nil,
			want:    "2001:db8::1",
		},
		{
			name:    "IPv6 zone is stripped",
			remote:  "[fe80::1%eth0]:1234",
			headers: nil,
			want:    "fe80::1",
		},
		{
			name:    "mapped trusted peer matches IPv4 prefix",
			remote:  "::ffff:10.0.0.5",
			headers: map[string][]string{"X-Forwarded-For": {"1.2.3.4"}},
			trusted: []string{"10.0.0.0/8"},
			want:    "1.2.3.4",
		},
		{
			name:    "invalid trusted configuration fails closed",
			remote:  "198.51.100.10:1234",
			headers: map[string][]string{"X-Forwarded-For": {"203.0.113.9"}},
			trusted: []string{"not-a-prefix"},
			want:    "198.51.100.10",
		},
		{
			name:    "unparseable peer returns empty",
			remote:  "not-an-ip",
			headers: map[string][]string{"X-Forwarded-For": {"1.2.3.4"}},
			trusted: []string{"10.0.0.0/8"},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://example.com/getIP", nil)
			r.RemoteAddr = tt.remote
			for header, values := range tt.headers {
				for _, value := range values {
					r.Header.Add(header, value)
				}
			}

			if got := ClientIP(r, tt.trusted); got != tt.want {
				t.Fatalf("ClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseTrustedProxies(t *testing.T) {
	tests := []struct {
		name    string
		entries []string
		want    []netip.Prefix
		wantErr string
	}{
		{
			name:    "empty entries are ignored",
			entries: nil,
			want:    []netip.Prefix{},
		},
		{
			name:    "whitespace and empty entries are ignored",
			entries: []string{"", "  ", "\t"},
			want:    []netip.Prefix{},
		},
		{
			name:    "CIDR entries",
			entries: []string{"10.0.0.0/8", " 127.0.0.1/32 ", "::1/128"},
			want: []netip.Prefix{
				netip.MustParsePrefix("10.0.0.0/8"),
				netip.MustParsePrefix("127.0.0.1/32"),
				netip.MustParsePrefix("::1/128"),
			},
		},
		{
			name:    "bare IP entries become single-host prefixes",
			entries: []string{"192.0.2.7", " 2001:db8::1 ", "::ffff:192.0.2.8"},
			want: []netip.Prefix{
				netip.MustParsePrefix("192.0.2.7/32"),
				netip.MustParsePrefix("2001:db8::1/128"),
				netip.MustParsePrefix("192.0.2.8/32"),
			},
		},
		{
			name:    "host bits are masked",
			entries: []string{"10.1.2.3/8"},
			want:    []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
		},
		{
			name:    "bad IP names the entry",
			entries: []string{"not-an-ip"},
			wantErr: `"not-an-ip"`,
		},
		{
			name:    "bad CIDR names the entry",
			entries: []string{"10.0.0.0/8", "10.0.0.0/33"},
			wantErr: `"10.0.0.0/33"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTrustedProxies(tt.entries)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ParseTrustedProxies() error = nil, want error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseTrustedProxies() error = %q, want it to contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTrustedProxies() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseTrustedProxies() = %v, want %v", got, tt.want)
			}
		})
	}
}
