package traceproxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
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

func TestProxyRoutesHostedUpstream(t *testing.T) {
	for _, tc := range []struct {
		name, base, path, want string
	}{
		{"origin", "", "/v1/chat/completions?model=a", "/v1/chat/completions?model=a"},
		{"versioned", "/v1", "/v1/chat/completions?model=a", "/v1/chat/completions?model=a"},
		{"nested base", "/api/v1/", "/v1/chat/a%2Fb?model=a", "/api/v1/chat/a%2Fb?model=a"},
		{"unrelated", "/v1", "/v10/models", "/v1/v10/models"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotHost, gotURI string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotHost, gotURI = r.Host, r.RequestURI
				w.WriteHeader(http.StatusNoContent)
			}))
			defer upstream.Close()
			target, err := url.Parse(upstream.URL + tc.base)
			if err != nil {
				t.Fatal(err)
			}
			events := make(channelSink, 1)
			server := httptest.NewServer(New(target, "support", "2", events))
			defer server.Close()
			resp, err := http.Get(server.URL + tc.path)
			if err != nil {
				t.Fatal(err)
			}
			_ = resp.Body.Close()
			<-events
			if gotHost != target.Host || gotURI != tc.want {
				t.Fatalf("upstream host/URI = %q %q, want %q %q", gotHost, gotURI, target.Host, tc.want)
			}
		})
	}
}

func TestProxyPreservesHeadersAndQueries(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer example" {
			t.Error("authorization was not forwarded")
		}
		if r.URL.Query().Get("base") != "yes" || r.URL.Query().Get("model") != "test" {
			t.Errorf("query parameters lost: %s", r.URL.RawQuery)
		}
		if r.Header.Get("X-Forwarded-Host") == "spoofed" || r.Header.Get("X-Forwarded-For") == "spoofed" {
			t.Error("untrusted forwarding headers survived")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL + "/v1?base=yes")
	if err != nil {
		t.Fatal(err)
	}
	events := make(channelSink, 1)
	server := httptest.NewServer(New(target, "support", "2", events))
	defer server.Close()
	req, err := http.NewRequest(http.MethodGet, server.URL+"/v1/models?model=test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer example")
	req.Header.Set("X-Forwarded-Host", "spoofed")
	req.Header.Set("X-Forwarded-For", "spoofed")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	<-events
}

func TestKafkaRESTSinkUsesConfluentEnvelope(t *testing.T) {
	received := make(chan map[string]any, 1)
	topic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/vnd.kafka.json.v2+json" {
			t.Errorf("unexpected content type %q", got)
		}
		raw, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(raw, &payload)
		received <- payload
		w.WriteHeader(http.StatusOK)
	}))
	defer topic.Close()
	sink := KafkaRESTSink{URL: topic.URL + "/topics/mlaiops.llm.traces", Client: topic.Client()}
	sink.Emit(Event{Agent: "support", SessionID: "sess-9", StatusCode: 200, CreatedAt: time.Now()})
	payload := <-received
	records, ok := payload["records"].([]any)
	if !ok || len(records) != 1 {
		t.Fatalf("expected one enveloped record, got %v", payload)
	}
	value := records[0].(map[string]any)["value"].(map[string]any)
	if value["agent"] != "support" || value["session_id"] != "sess-9" {
		t.Fatalf("unexpected event value: %v", value)
	}
}
