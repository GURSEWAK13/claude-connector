package peer

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	headerSignature = "X-Claude-Signature"
	headerTimestamp = "X-Claude-Timestamp"
	headerNonce     = "X-Claude-Nonce"

	timestampWindow = 30 * time.Second
	nonceCacheSize  = 1024
)

// Authenticator signs and verifies peer requests using HMAC-SHA256.
type Authenticator struct {
	secret []byte

	// Nonce LRU cache to prevent replays
	nonceMu   sync.Mutex
	nonceList []string
	nonceSet  map[string]struct{}
}

func NewAuthenticator(secret string) *Authenticator {
	return &Authenticator{
		secret:   []byte(secret),
		nonceSet: make(map[string]struct{}, nonceCacheSize),
	}
}

// Sign adds authentication headers to an outgoing request.
func (a *Authenticator) Sign(req *http.Request, bodyHash string) error {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	nonce, err := generateNonce()
	if err != nil {
		return fmt.Errorf("generate nonce: %w", err)
	}

	msg := buildMessage(req.Method, req.URL.Path, timestamp, nonce, bodyHash)
	sig := a.sign(msg)

	req.Header.Set(headerTimestamp, timestamp)
	req.Header.Set(headerNonce, nonce)
	req.Header.Set(headerSignature, sig)
	return nil
}

// Verify checks authentication headers on an incoming request.
func (a *Authenticator) Verify(req *http.Request, bodyHash string) error {
	if len(a.secret) == 0 {
		return nil // No secret configured; skip auth
	}

	timestamp := req.Header.Get(headerTimestamp)
	nonce := req.Header.Get(headerNonce)
	sig := req.Header.Get(headerSignature)

	if timestamp == "" || nonce == "" || sig == "" {
		return fmt.Errorf("missing auth headers")
	}

	// Check timestamp window
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp")
	}
	age := time.Since(time.Unix(ts, 0))
	if age > timestampWindow || age < -timestampWindow {
		return fmt.Errorf("timestamp out of window (age: %v)", age)
	}

	// Check nonce uniqueness
	if !a.checkNonce(nonce) {
		return fmt.Errorf("nonce already used (replay detected)")
	}

	// Verify signature
	msg := buildMessage(req.Method, req.URL.Path, timestamp, nonce, bodyHash)
	expected := a.sign(msg)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

func (a *Authenticator) sign(msg string) string {
	mac := hmac.New(sha256.New, a.secret)
	mac.Write([]byte(msg))
	return hex.EncodeToString(mac.Sum(nil))
}

func buildMessage(method, path, timestamp, nonce, bodyHash string) string {
	return method + "\n" + path + "\n" + timestamp + "\n" + nonce + "\n" + bodyHash
}

// HashBody returns the hex-encoded SHA-256 of the body bytes.
func HashBody(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func generateNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// checkNonce returns true if the nonce is fresh (not seen before), and records it.
func (a *Authenticator) checkNonce(nonce string) bool {
	a.nonceMu.Lock()
	defer a.nonceMu.Unlock()

	if _, seen := a.nonceSet[nonce]; seen {
		return false
	}

	// Add to cache
	a.nonceSet[nonce] = struct{}{}
	a.nonceList = append(a.nonceList, nonce)

	// Evict oldest if over capacity
	for len(a.nonceList) > nonceCacheSize {
		oldest := a.nonceList[0]
		a.nonceList = a.nonceList[1:]
		delete(a.nonceSet, oldest)
	}

	return true
}
