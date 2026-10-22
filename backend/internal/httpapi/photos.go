package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"shiguang-island/backend/internal/store"
)

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.store.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "数据库暂时不可用")
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}

func (h *Handler) listPhotos(w http.ResponseWriter, r *http.Request) {
	limit := clampInt(r.URL.Query().Get("limit"), 24, 1, 60)
	cursor, err := parseID(r.URL.Query().Get("cursor"), true)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor 必须是正整数")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	photos, err := h.store.ListPhotos(ctx, h.clientID(r), cursor, limit)
	if err != nil {
		h.logger.Error("查询照片失败", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "照片加载失败，请稍后重试")
		return
	}
	writeData(w, http.StatusOK, map[string]any{
		"items":      photos,
		"nextCursor": nextCursor(len(photos), limit, lastPhotoID(photos)),
	})
}

func (h *Handler) likePhoto(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "照片 ID 不合法")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	likes, liked, err := h.store.LikePhoto(ctx, id, h.clientID(r))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "这张照片已经不在岛上了")
		return
	}
	if err != nil {
		h.logger.Error("点赞失败", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "点赞失败，请稍后重试")
		return
	}
	writeData(w, http.StatusOK, map[string]any{"likes": likes, "liked": liked})
}

func (h *Handler) adminDeletePhoto(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "照片 ID 不合法")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	url, err := h.store.DeletePhoto(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "照片不存在")
		return
	}
	if err != nil {
		h.logger.Error("删除照片失败", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "删除失败")
		return
	}
	h.removeUploadedFile(url)
	w.WriteHeader(http.StatusNoContent)
}

func lastPhotoID(items []store.Photo) int64 {
	if len(items) == 0 {
		return 0
	}
	return items[len(items)-1].ID
}

func cleanNickname(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "匿名星友", nil
	}
	if utf8.RuneCountInString(s) > 24 {
		return "", errors.New("昵称最多 24 个字")
	}
	return s, nil
}

func cleanText(raw string, maxRunes int, required bool) (string, error) {
	s := strings.TrimSpace(strings.ReplaceAll(raw, "\r\n", "\n"))
	if s == "" && required {
		return "", errors.New("内容不能为空")
	}
	if utf8.RuneCountInString(s) > maxRunes {
		return "", errors.New("内容超出长度限制")
	}
	return s, nil
}

func parseID(raw string, allowEmpty bool) (int64, error) {
	if strings.TrimSpace(raw) == "" {
		if allowEmpty {
			return 0, nil
		}
		return 0, errors.New("id 不能为空")
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("id 必须是正整数")
	}
	return id, nil
}

func clampInt(raw string, fallback, min, max int) int {
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func nextCursor(length, limit int, lastID int64) int64 {
	if length < limit {
		return 0
	}
	return lastID
}
