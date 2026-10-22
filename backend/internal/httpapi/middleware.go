package httpapi

import (
	"crypto/subtle"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (h *Handler) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)
		h.logger.Info("http",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", ww.status),
			slog.Duration("duration", time.Since(start)),
			slog.String("ip", h.clientIP(r)),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (h *Handler) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.cfg.AdminToken == "" {
			writeError(w, http.StatusServiceUnavailable, "admin_disabled", "管理端未启用，请先配置 ADMIN_TOKEN")
			return
		}
		token := strings.TrimSpace(r.Header.Get("X-Admin-Token"))
		if token == "" {
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				token = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
			}
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(h.cfg.AdminToken)) != 1 {
			writeError(w, http.StatusUnauthorized, "unauthorized", "管理令牌无效")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) clientIP(r *http.Request) string {
	if h.cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if first, _, ok := strings.Cut(xff, ","); ok {
				return strings.TrimSpace(first)
			}
			return strings.TrimSpace(xff)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// clientID 用浏览器生成的 UUID 做点赞去重；缺失时退化为 IP，保证幂等。
func (h *Handler) clientID(r *http.Request) string {
	id := strings.TrimSpace(r.Header.Get("X-Client-Id"))
	if id != "" && len(id) <= 64 && isPrintableASCII(id) {
		return id
	}
	return "ip:" + h.clientIP(r)
}

func isPrintableASCII(s string) bool {
	for _, r := range s {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

// rateLimiter 是进程内固定窗口限流器，够用且无额外依赖。
// 多实例部署时每实例独立计数，如需全局限流可替换为 Redis。
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	period  time.Duration
	entries map[string]*rateEntry
	lastGC  time.Time
}

type rateEntry struct {
	count int
	reset time.Time
}

func newRateLimiter(limit int, period time.Duration) *rateLimiter {
	return &rateLimiter{
		limit:   limit,
		period:  period,
		entries: make(map[string]*rateEntry),
		lastGC:  time.Now(),
	}
}

func (l *rateLimiter) allow(key string) (bool, int) {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if now.Sub(l.lastGC) > 10*time.Minute || len(l.entries) > 20000 {
		for k, e := range l.entries {
			if now.After(e.reset) {
				delete(l.entries, k)
			}
		}
		l.lastGC = now
	}

	e, ok := l.entries[key]
	if !ok || now.After(e.reset) {
		l.entries[key] = &rateEntry{count: 1, reset: now.Add(l.period)}
		return true, l.limit - 1
	}
	if e.count >= l.limit {
		retry := int(e.reset.Sub(now).Seconds()) + 1
		return false, retry
	}
	e.count++
	return true, l.limit - e.count
}

func (h *Handler) rateLimit(limiter *rateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ok, remaining := limiter.allow(h.clientIP(r))
			if !ok {
				w.Header().Set("Retry-After", strconv.Itoa(remaining))
				writeError(w, http.StatusTooManyRequests, "rate_limited", "发得太快啦，休息一下再试")
				return
			}
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			next.ServeHTTP(w, r)
		})
	}
}
