package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type User struct {
	ID        int64  `json:"id"`
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

type Chat struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

type Update struct {
	ID      int64 `json:"update_id"`
	Message *struct {
		From User `json:"from"`
		Chat Chat `json:"chat"`
	} `json:"message"`
}

type apiResponse[T any] struct {
	OK          bool   `json:"ok"`
	Result      T      `json:"result"`
	Description string `json:"description"`
}

func New(baseURL, token string, timeout time.Duration) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, errors.New("invalid telegram API URL")
	}
	if token == "" || len(token) > 256 || strings.ContainsAny(token, "/\r\n") {
		return nil, errors.New("invalid Telegram token")
	}
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		ForceAttemptHTTP2:   true,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil || len(addresses) == 0 {
				return nil, errors.New("telegram host resolution failed")
			}
			for _, candidate := range addresses {
				private := candidate.IP.IsPrivate() || candidate.IP.IsLinkLocalUnicast() || candidate.IP.IsUnspecified()
				if private || candidate.IP.IsLoopback() {
					if parsed.Scheme != "http" || !candidate.IP.IsLoopback() {
						continue
					}
				}
				return dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
			}
			return nil, errors.New("telegram destination is not allowed")
		},
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), token: token, http: &http.Client{Timeout: timeout, Transport: transport}}, nil
}

func (c *Client) GetMe(ctx context.Context) (User, error) {
	return call[User](ctx, c, "getMe", nil)
}

func (c *Client) GetUpdates(ctx context.Context) ([]Update, error) {
	return call[[]Update](ctx, c, "getUpdates", map[string]any{"limit": 100, "timeout": 0, "allowed_updates": []string{"message"}})
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) error {
	if chatID == 0 || strings.TrimSpace(text) == "" || len(text) > 4096 {
		return errors.New("invalid Telegram message")
	}
	_, err := call[json.RawMessage](ctx, c, "sendMessage", map[string]any{"chat_id": strconv.FormatInt(chatID, 10), "text": text})
	return err
}

func call[T any](ctx context.Context, client *Client, method string, payload any) (T, error) {
	var zero T
	body, err := json.Marshal(payload)
	if err != nil {
		return zero, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf("%s/bot%s/%s", client.baseURL, client.token, method), bytes.NewReader(body))
	if err != nil {
		return zero, err
	}
	request.Header.Set("Content-Type", "application/json")
	httpResponse, err := client.http.Do(request)
	if err != nil {
		return zero, errors.New("telegram API network request failed")
	}
	defer func() { _ = httpResponse.Body.Close() }()
	limited := io.LimitReader(httpResponse.Body, 1<<20)
	var envelope apiResponse[T]
	if err := json.NewDecoder(limited).Decode(&envelope); err != nil {
		return zero, errors.New("invalid telegram API response")
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 || !envelope.OK {
		return zero, fmt.Errorf("telegram API request failed: %s", envelope.Description)
	}
	return envelope.Result, nil
}
