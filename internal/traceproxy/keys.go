package traceproxy

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// APIKey is one inbound credential. Only the SHA-256 of the key is stored.
type APIKey struct {
	// ID names the key in traces and logs; it is never the key itself.
	ID string `json:"key_id"`
	// SHA256 is the lowercase hex SHA-256 of the key.
	SHA256 string `json:"key_sha256"`
	// Project the key's calls are attributed to.
	Project string `json:"project"`
	// Agent optionally overrides MLAIOPS_AGENT_NAME for this key's traces.
	Agent string `json:"agent,omitempty"`

	hash []byte
}

// KeyStore authenticates inbound requests against hashed project keys.
type KeyStore struct{ keys []APIKey }

var (
	keyIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	hashPattern  = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// ParseKeys reads TRACE_PROXY_KEYS: a JSON array of
// {"key_id", "key_sha256", "project", "agent"} objects.
func ParseKeys(raw string) (*KeyStore, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var keys []APIKey
	if err := decoder.Decode(&keys); err != nil {
		return nil, errors.New("TRACE_PROXY_KEYS must be a JSON array of {key_id, key_sha256, project, agent} objects")
	}
	if len(keys) == 0 {
		return nil, errors.New("TRACE_PROXY_KEYS lists no keys")
	}
	ids := map[string]bool{}
	hashes := map[string]bool{}
	for i := range keys {
		key := &keys[i]
		key.SHA256 = strings.ToLower(strings.TrimSpace(key.SHA256))
		switch {
		case !keyIDPattern.MatchString(key.ID):
			return nil, fmt.Errorf("TRACE_PROXY_KEYS entry %d: key_id must be 1-128 letters, digits, '.', '_' or '-'", i)
		case !hashPattern.MatchString(key.SHA256):
			return nil, fmt.Errorf("TRACE_PROXY_KEYS entry %s: key_sha256 must be 64 hex characters", key.ID)
		case strings.TrimSpace(key.Project) == "":
			return nil, fmt.Errorf("TRACE_PROXY_KEYS entry %s: project is required", key.ID)
		case ids[key.ID]:
			return nil, fmt.Errorf("TRACE_PROXY_KEYS lists key_id %s twice", key.ID)
		case hashes[key.SHA256]:
			return nil, fmt.Errorf("TRACE_PROXY_KEYS entry %s repeats another key's hash", key.ID)
		}
		ids[key.ID], hashes[key.SHA256] = true, true
		key.hash, _ = hex.DecodeString(key.SHA256)
	}
	return &KeyStore{keys: keys}, nil
}

// HashKey returns the key_sha256 value for a key.
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// credential extracts the caller's key: "Authorization: Bearer <key>" (OpenAI
// clients), "x-api-key" (Anthropic clients) or "api-key" (Azure clients).
func credential(header http.Header) string {
	if value := header.Get("Authorization"); value != "" {
		scheme, token, ok := strings.Cut(value, " ")
		if ok && strings.EqualFold(scheme, "Bearer") {
			return strings.TrimSpace(token)
		}
		return ""
	}
	if value := header.Get("X-Api-Key"); value != "" {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(header.Get("Api-Key"))
}

// Authenticate returns the key matching the request's credential. Every
// stored hash is compared in constant time, so timing does not reveal which
// key, if any, nearly matched.
func (s *KeyStore) Authenticate(header http.Header) (APIKey, bool) {
	presented := credential(header)
	if presented == "" {
		return APIKey{}, false
	}
	sum := sha256.Sum256([]byte(presented))
	match := -1
	for i := range s.keys {
		if subtle.ConstantTimeCompare(sum[:], s.keys[i].hash) == 1 {
			match = i
		}
	}
	if match < 0 {
		return APIKey{}, false
	}
	return s.keys[match], true
}

var invalidKeyBody = func() []byte {
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(map[string]any{"error": map[string]any{
		"message": "Invalid or missing API key. Send your Kionga project key as 'Authorization: Bearer <key>' or 'x-api-key'.",
		"type":    "invalid_api_key",
		"code":    "invalid_api_key",
		"param":   nil,
	}})
	return body.Bytes()
}()

func writeInvalidKey(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", `Bearer realm="kionga-trace-proxy"`)
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write(bytes.Clone(invalidKeyBody))
}
