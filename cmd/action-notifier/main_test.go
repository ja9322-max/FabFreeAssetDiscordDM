package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnseen(t *testing.T) {
	current := []asset{{Name: "Old", Link: "old"}, {Name: "New", Link: "new"}}
	got := unseen(current, []string{"old"})
	if len(got) != 1 || got[0].Link != "new" {
		t.Fatalf("unseen() = %#v", got)
	}
}

func TestSendDM(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bot secret" {
			t.Fatalf("missing bot authorization")
		}
		switch r.URL.Path {
		case "/users/@me/channels":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["recipient_id"] != "42" {
				t.Fatalf("recipient_id = %q", body["recipient_id"])
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"dm-1"}`))
		case "/channels/dm-1/messages":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["content"] != "hello" {
				t.Fatalf("content = %q", body["content"])
			}
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &http.Client{Transport: rewriteTransport{base: server.URL, transport: http.DefaultTransport}}
	if err := sendDM(context.Background(), client, "secret", "42", "hello"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d", calls)
	}
}

type rewriteTransport struct {
	base      string
	transport http.RoundTripper
}

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = strings.TrimSuffix(strings.TrimPrefix(r.base, "http"), "s")
	if strings.HasPrefix(r.base, "https") {
		req.URL.Scheme = "https"
	} else {
		req.URL.Scheme = "http"
	}
	req.URL.Host = strings.TrimPrefix(strings.TrimPrefix(r.base, "https://"), "http://")
	return r.transport.RoundTrip(req)
}
