package updater

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		left, right string
		want        int
	}{
		{"v1.1.0", "v1.0.9", 1},
		{"v1.0.0", "v1.0.0", 0},
		{"v1.9.9", "v2.0.0", -1},
		{"v2.0.0-rc.1", "v1.9.0", 1},
	}
	for _, test := range tests {
		if got := compareVersions(test.left, test.right); got != test.want {
			t.Fatalf("compareVersions(%q, %q)=%d want %d", test.left, test.right, got, test.want)
		}
	}
}

func TestHTTPCheckerValidatesReleaseResponse(t *testing.T) {
	checker := &HTTPChecker{client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != latestReleaseURL || request.Header.Get("User-Agent") != "mini-ubuntu-server" {
			t.Fatal("unexpected release request")
		}
		body := `{"tag_name":"v1.2.0","html_url":"https://github.com/kantaevsherhan/mini-ubuntu-server-panel/releases/tag/v1.2.0","draft":false}`
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	})}}
	status, err := checker.Check(context.Background(), "v1.1.0")
	if err != nil || !status.Available || status.Latest != "v1.2.0" {
		t.Fatalf("valid release rejected: %#v %v", status, err)
	}
}
