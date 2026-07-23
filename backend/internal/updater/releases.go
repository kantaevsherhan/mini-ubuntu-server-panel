package updater

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const latestReleaseURL = "https://api.github.com/repos/kantaevsherhan/mini-ubuntu-server-panel/releases/latest"

var versionPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

type Status struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	URL       string `json:"url"`
}

type Checker interface {
	Check(context.Context, string) (Status, error)
}

type HTTPChecker struct {
	client *http.Client
}

func NewHTTPChecker() *HTTPChecker {
	return &HTTPChecker{client: &http.Client{Timeout: 10 * time.Second}}
}

func (c *HTTPChecker) Check(ctx context.Context, current string) (Status, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return Status{}, errors.New("failed to create release request")
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "mini-ubuntu-server")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := c.client.Do(request)
	if err != nil {
		return Status{}, errors.New("release service unavailable")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return Status{}, errors.New("release service returned an error")
	}
	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Draft   bool   `json:"draft"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64*1024))
	if decoder.Decode(&payload) != nil || payload.Draft || !versionPattern.MatchString(payload.TagName) {
		return Status{}, errors.New("release response is invalid")
	}
	if !strings.HasPrefix(payload.HTMLURL, "https://github.com/kantaevsherhan/mini-ubuntu-server-panel/releases/") {
		return Status{}, errors.New("release URL is invalid")
	}
	return Status{Current: current, Latest: payload.TagName, Available: compareVersions(payload.TagName, current) > 0, URL: payload.HTMLURL}, nil
}

func compareVersions(left, right string) int {
	parse := func(value string) [3]int {
		value = strings.TrimPrefix(strings.SplitN(value, "-", 2)[0], "v")
		var result [3]int
		for index, part := range strings.Split(value, ".") {
			if index >= len(result) {
				break
			}
			result[index], _ = strconv.Atoi(part)
		}
		return result
	}
	l, r := parse(left), parse(right)
	for index := range l {
		if l[index] > r[index] {
			return 1
		}
		if l[index] < r[index] {
			return -1
		}
	}
	return 0
}
