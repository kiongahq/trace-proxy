package traceproxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

type channelSink chan Event

func (s channelSink) Emit(event Event) { s <- event }

func TestProxyForwardsAndEmitsTrace(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-MLAIOps-Agent") != "support" {
			t.Error("agent header missing")
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	events := make(channelSink, 1)
	server := httptest.NewServer(New(target, "support", "2", events))
	defer server.Close()
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", nil)
	req.Header.Set("X-Session-ID", "session-1")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	event := <-events
	if response.StatusCode != http.StatusCreated || event.SessionID != "session-1" || event.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected response/event: %d %#v", response.StatusCode, event)
	}
}
