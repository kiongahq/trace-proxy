package traceproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Event struct {
	Agent   string `json:"agent"`
	Version string `json:"version"`
	// Project and KeyID identify the authenticated caller. The key itself is
	// never recorded.
	Project    string    `json:"project,omitempty"`
	KeyID      string    `json:"key_id,omitempty"`
	SessionID  string    `json:"session_id"`
	Model      string    `json:"model,omitempty"`
	StatusCode int       `json:"status_code"`
	DurationMS int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

type Sink interface{ Emit(Event) }

type HTTPSink struct {
	URL, Token string
	Client     *http.Client
}

func (s HTTPSink) Emit(event Event) {
	if s.URL == "" {
		return
	}
	raw, _ := json.Marshal(event)
	req, err := http.NewRequest(http.MethodPost, s.URL, bytes.NewReader(raw))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if s.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.Token)
	}
	response, err := s.Client.Do(req)
	if err == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}
}

// KafkaRESTSink publishes events to the Karapace Kafka REST endpoint using its
// Confluent-compatible envelope, e.g. http://kafka-rest:8082/topics/mlaiops.llm.traces.
type KafkaRESTSink struct {
	URL    string
	Client *http.Client
}

func (s KafkaRESTSink) Emit(event Event) {
	if s.URL == "" {
		return
	}
	raw, _ := json.Marshal(map[string]any{"records": []map[string]any{{"value": event}}})
	req, err := http.NewRequest(http.MethodPost, s.URL, bytes.NewReader(raw))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/vnd.kafka.json.v2+json")
	response, err := s.Client.Do(req)
	if err == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
	}
}

// Config configures the proxy handler.
type Config struct {
	Upstream       *url.URL
	Agent, Version string
	Sink           Sink
	// Keys authenticates inbound requests. Required unless AllowUnauthenticated.
	Keys *KeyStore
	// AllowUnauthenticated serves without inbound keys and forwards the
	// client's own credentials. Development only; it must be chosen explicitly.
	AllowUnauthenticated bool
	// UpstreamCredential is set on every authenticated upstream request in
	// place of the caller's credentials. Empty sends no credential.
	UpstreamCredential UpstreamCredential
}

// UpstreamCredential is the provider credential the proxy injects.
type UpstreamCredential struct {
	// Header is the header name: Authorization (OpenAI and compatible),
	// x-api-key (Anthropic) or api-key (Azure OpenAI).
	Header string
	// Value is the full header value, for example "Bearer sk-...".
	Value string
}

var headerName = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

// NewUpstreamCredential builds the injected credential from the provider key,
// header name and scheme. The scheme defaults to Bearer for Authorization and
// to none for any other header.
func NewUpstreamCredential(key, header, scheme string) (UpstreamCredential, error) {
	key, header, scheme = strings.TrimSpace(key), strings.TrimSpace(header), strings.TrimSpace(scheme)
	if key == "" {
		return UpstreamCredential{}, nil
	}
	if header == "" {
		header = "Authorization"
	}
	if !headerName.MatchString(header) {
		return UpstreamCredential{}, errors.New("LLM_UPSTREAM_AUTH_HEADER is not a valid header name")
	}
	if strings.EqualFold(header, "Authorization") && scheme == "" {
		scheme = "Bearer"
	}
	if strings.ContainsAny(key+scheme, "\r\n") || strings.ContainsAny(scheme, " \t") {
		return UpstreamCredential{}, errors.New("LLM_UPSTREAM_API_KEY and LLM_UPSTREAM_AUTH_SCHEME must be single-line values")
	}
	value := key
	if scheme != "" {
		value = scheme + " " + key
	}
	return UpstreamCredential{Header: http.CanonicalHeaderKey(header), Value: value}, nil
}

// clientCredentialHeaders are removed from every authenticated request: they
// carry the caller's Kionga key, which must never reach the provider.
var clientCredentialHeaders = []string{"Authorization", "Api-Key", "X-Api-Key", "Proxy-Authorization"}

type callerKey struct{}

// New returns the proxy handler. It refuses a configuration without inbound
// keys unless AllowUnauthenticated is set.
func New(cfg Config) (http.Handler, error) {
	if cfg.Upstream == nil || cfg.Upstream.Scheme == "" || cfg.Upstream.Host == "" {
		return nil, errors.New("LLM_UPSTREAM_URL must be an absolute URL")
	}
	if cfg.Keys == nil && !cfg.AllowUnauthenticated {
		return nil, errors.New("trace-proxy requires TRACE_PROXY_KEYS (or TRACE_PROXY_KEYS_FILE); set TRACE_PROXY_ALLOW_UNAUTHENTICATED=true only for local development")
	}
	if cfg.Keys != nil && cfg.AllowUnauthenticated {
		return nil, errors.New("set only one of TRACE_PROXY_KEYS and TRACE_PROXY_ALLOW_UNAUTHENTICATED")
	}
	if cfg.AllowUnauthenticated && cfg.UpstreamCredential.Value != "" {
		// Injecting the provider key without inbound authentication would make
		// the proxy an open relay for that key.
		return nil, errors.New("LLM_UPSTREAM_API_KEY requires TRACE_PROXY_KEYS; it cannot be used with TRACE_PROXY_ALLOW_UNAUTHENTICATED")
	}
	credential := cfg.UpstreamCredential
	if cfg.Sink == nil {
		cfg.Sink = HTTPSink{}
	}
	upstream, agent, version, sink := cfg.Upstream, cfg.Agent, cfg.Version, cfg.Sink
	proxy := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
		target := *upstream
		base := strings.TrimSuffix(target.EscapedPath(), "/")
		path := r.In.URL.EscapedPath()
		if strings.HasSuffix(base, "/v1") && (path == "/v1" || strings.HasPrefix(path, "/v1/")) {
			target.Path = strings.TrimSuffix(strings.TrimSuffix(target.Path, "/"), "/v1")
			if target.RawPath != "" {
				target.RawPath = strings.TrimSuffix(base, "/v1")
			}
		}
		r.SetURL(&target)
		r.SetXForwarded()
		callerAgent := agent
		if key, ok := r.In.Context().Value(callerKey{}).(APIKey); ok {
			for _, header := range clientCredentialHeaders {
				r.Out.Header.Del(header)
			}
			if credential.Value != "" {
				r.Out.Header.Set(credential.Header, credential.Value)
			}
			if key.Agent != "" {
				callerAgent = key.Agent
			}
		}
		r.Out.Header.Set("X-MLAIOps-Agent", callerAgent)
		r.Out.Header.Set("X-MLAIOps-Agent-Version", version)
	}}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
			return
		}
		event := Event{Agent: agent, Version: version, SessionID: r.Header.Get("X-Session-ID")}
		if cfg.Keys != nil {
			key, ok := cfg.Keys.Authenticate(r.Header)
			if !ok {
				writeInvalidKey(w)
				return
			}
			event.Project, event.KeyID = key.Project, key.ID
			if key.Agent != "" {
				event.Agent = key.Agent
			}
			r = r.WithContext(context.WithValue(r.Context(), callerKey{}, key))
		}
		start := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		proxy.ServeHTTP(recorder, r)
		event.StatusCode, event.DurationMS, event.CreatedAt = recorder.status, time.Since(start).Milliseconds(), time.Now().UTC()
		go sink.Emit(event)
	}), nil
}

// statusRecorder captures the status the client receives. It exposes Flush and
// Unwrap so httputil.ReverseProxy (through http.ResponseController) can flush
// each server-sent event to the client as soon as the upstream writes it.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *statusRecorder) WriteHeader(status int) {
	if !w.wroteHeader {
		w.status = status
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.status = http.StatusOK
		w.wroteHeader = true
	}
	return w.ResponseWriter.Write(body)
}

// Flush sends buffered data to the client when the underlying writer can.
func (w *statusRecorder) Flush() {
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

// Unwrap lets http.ResponseController reach the underlying writer for the
// features statusRecorder does not wrap (deadlines, hijacking, full duplex).
func (w *statusRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }
