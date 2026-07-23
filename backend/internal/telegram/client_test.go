package telegram

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientGetMeAndSendMessage(t *testing.T) {
	requests := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.URL.Path)
		writer.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(request.URL.Path, "/getMe") {
			fmt.Fprint(writer, `{"ok":true,"result":{"id":42,"username":"panel_bot"}}`)
			return
		}
		fmt.Fprint(writer, `{"ok":true,"result":{"message_id":1}}`)
	}))
	defer server.Close()
	client, err := New(server.URL, "123:test-token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	bot, err := client.GetMe(context.Background())
	if err != nil || bot.Username != "panel_bot" {
		t.Fatalf("unexpected bot response: %#v, %v", bot, err)
	}
	if err := client.SendMessage(context.Background(), -100123, "hello"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || strings.Contains(strings.Join(requests, " "), "hello") {
		t.Fatalf("unexpected requests: %#v", requests)
	}
}

func TestClientRejectsPrivateHTTPSDestination(t *testing.T) {
	client, err := New("https://127.0.0.1:8443", "123:test-token", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetMe(context.Background()); err == nil {
		t.Fatal("expected private HTTPS destination to be rejected")
	}
}
