package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"shiguang-island/backend/internal/config"
	"shiguang-island/backend/internal/store"
)

type Handler struct {
	cfg           config.Config
	store         *store.Store
	logger        *slog.Logger
	static        http.Handler
	writeLimiter  *rateLimiter
	uploadLimiter *rateLimiter
}

// New 组装路由。static 为 embed 出来的前端文件服务，未提供时根路径返回 404。
func New(cfg config.Config, st *store.Store, logger *slog.Logger, static http.Handler) http.Handler {
	h := &Handler{
		cfg:           cfg,
		store:         st,
		logger:        logger,
		static:        static,
		writeLimiter:  newRateLimiter(30, time.Minute),
		uploadLimiter: newRateLimiter(10, time.Minute),
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(h.logRequests)
	r.Use(h.securityHeaders)

	r.Handle("/uploads/*", http.HandlerFunc(h.serveUpload))

	r.Route("/api", func(api chi.Router) {
		api.Get("/health", h.health)

		api.Get("/photos", h.listPhotos)
		api.With(h.rateLimit(h.uploadLimiter)).Post("/photos", h.createPhoto)
		api.With(h.rateLimit(h.writeLimiter)).Post("/photos/{id}/like", h.likePhoto)

		api.Get("/messages", h.listMessages)
		api.With(h.rateLimit(h.writeLimiter)).Post("/messages", h.createMessage)
		api.Get("/messages/{id}/replies", h.listReplies)
		api.With(h.rateLimit(h.writeLimiter)).Post("/messages/{id}/replies", h.createReply)

		api.Route("/admin", func(admin chi.Router) {
			admin.Use(h.requireAdmin)
			admin.Get("/ping", h.adminPing)
			admin.Delete("/photos/{id}", h.adminDeletePhoto)
			admin.Delete("/messages/{id}", h.adminDeleteMessage)
			admin.Delete("/replies/{id}", h.adminDeleteReply)
		})

		api.NotFound(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotFound, "not_found", "接口不存在")
		})
		api.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不被允许")
		})
	})

	if h.static != nil {
		r.NotFound(h.static.ServeHTTP)
	} else {
		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		})
	}
	return r
}
