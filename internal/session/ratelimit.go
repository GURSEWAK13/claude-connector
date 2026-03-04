package session

import (
	"net/http"
	"strconv"
	"time"
)

// ParseRetryAfter extracts the retry-after duration from a 429 response.
func ParseRetryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}

	// Try Retry-After header (seconds or HTTP date)
	val := resp.Header.Get("Retry-After")
	if val == "" {
		val = resp.Header.Get("X-RateLimit-Reset-After")
	}
	if val == "" {
		val = resp.Header.Get("X-Ratelimit-Reset-Requests")
	}

	if val != "" {
		// Try as integer seconds
		if secs, err := strconv.ParseFloat(val, 64); err == nil {
			return time.Duration(secs * float64(time.Second))
		}
		// Try as HTTP date
		if t, err := http.ParseTime(val); err == nil {
			d := time.Until(t)
			if d > 0 {
				return d
			}
		}
	}

	return 0
}

// IsRateLimitResponse returns true if this HTTP response indicates rate limiting.
func IsRateLimitResponse(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests
}
