// Copyright (C) 2026 Fastah Inc.

package main

import (
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openrdap/rdap"
)

func TestPrefixesForRDAPNetwork(t *testing.T) {
	tests := []struct {
		name    string
		network *rdap.IPNetwork
		want    []string
	}{
		{
			name: "ipv4 cidr range",
			network: &rdap.IPNetwork{
				StartAddress: "192.0.2.0",
				EndAddress:   "192.0.2.255",
			},
			want: []string{"192.0.2.0/24"},
		},
		{
			name: "ipv4 non cidr range",
			network: &rdap.IPNetwork{
				StartAddress: "192.0.2.1",
				EndAddress:   "192.0.2.3",
			},
			want: []string{"192.0.2.1/32", "192.0.2.2/31"},
		},
		{
			name: "ipv6 cidr range",
			network: &rdap.IPNetwork{
				StartAddress: "2001:db8::",
				EndAddress:   "2001:db8::ffff",
			},
			want: []string{"2001:db8::/112"},
		},
		{
			name: "mixed families are invalid",
			network: &rdap.IPNetwork{
				StartAddress: "192.0.2.1",
				EndAddress:   "2001:db8::1",
			},
		},
		{
			name: "start after end is invalid",
			network: &rdap.IPNetwork{
				StartAddress: "192.0.2.3",
				EndAddress:   "192.0.2.1",
			},
		},
		{
			name: "nil network is invalid",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := prefixStrings(prefixesForRDAPNetwork(test.network))
			if len(got) != len(test.want) {
				t.Fatalf("prefixesForRDAPNetwork() = %v, want %v", got, test.want)
			}
			for index := range test.want {
				if got[index] != test.want[index] {
					t.Fatalf("prefixesForRDAPNetwork() = %v, want %v", got, test.want)
				}
			}
		})
	}
}

func prefixStrings(prefixes []netip.Prefix) []string {
	if prefixes == nil {
		return nil
	}
	values := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		values = append(values, prefix.String())
	}
	return values
}

func TestHTTPCacheTransportUsesDiskCache(t *testing.T) {
	var calls atomic.Int32
	transport, err := newHTTPCacheTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		call := calls.Add(1)
		return stringResponse(req, http.StatusOK, fmt.Sprintf("body-%d", call)), nil
	}), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	first := readResponseBody(t, transport, "https://rdap.example.test/ip/192.0.2.1")
	second := readResponseBody(t, transport, "https://rdap.example.test/ip/192.0.2.1")
	if first != "body-1" || second != "body-1" {
		t.Fatalf("cached bodies = %q, %q; want both body-1", first, second)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("underlying transport calls = %d, want 1", got)
	}

	stats := transport.stats()
	if stats.DiskHits != 1 || stats.Misses != 1 || stats.MemoryHits != 0 {
		t.Fatalf("stats = %+v, want one disk hit, one miss, zero memory hits", stats)
	}
}

func TestHTTPCacheTransportEvictsNonSuccessResponses(t *testing.T) {
	var calls atomic.Int32
	transport, err := newHTTPCacheTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		call := calls.Add(1)
		return stringResponse(req, http.StatusNotFound, fmt.Sprintf("missing-%d", call)), nil
	}), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	first := readResponseBody(t, transport, "https://rdap.example.test/ip/192.0.2.404")
	second := readResponseBody(t, transport, "https://rdap.example.test/ip/192.0.2.404")
	if first != "missing-1" || second != "missing-2" {
		t.Fatalf("non-success bodies = %q, %q; want fresh responses", first, second)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("underlying transport calls = %d, want 2", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func readResponseBody(t *testing.T, transport http.RoundTripper, rawURL string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func stringResponse(req *http.Request, statusCode int, body string) *http.Response {
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode)),
		StatusCode:    statusCode,
		Header:        make(http.Header),
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}
}
