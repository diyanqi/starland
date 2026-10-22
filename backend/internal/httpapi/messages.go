package httpapi

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"shiguang-island/backend/internal/store"
)

var allowedMessageTypes = map[string]bool{
	"praise":  true,
	"rant":    true,
	"thought": true,
}

type createMessageRequest struct {
	Content  string `json:"content"`
	Type     string `json:"type"`
	Nickname string `json:"nickname"`
}

type createReplyRequest struct {
	Content  string `json:"content"`
	Nickname string `json:"nickname"`
}

func (h *Handler) listMessages(w http.ResponseWriter, r *http.Request) {
	msgType := r.URL.Query().Get("type")
	if msgType != "" && !allowedMessageTypes[msgType] {
		writeError(w, http.StatusBadRequest, "invalid_type", "消息类型只能是 praise、rant 或 thought")
		return
	}
	limit := clampInt(r.URL.Query().Get("limit"), 20, 1, 50)
	cursor, err := parseID(r.URL.Query().Get("cursor"), true)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_cursor", "cursor 必须是正整数")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	messages, err := h.store.ListMessages(ctx, msgType, cursor, limit)
	if err != nil {
		h.logger.Error("查询留言失败", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "文字岛加载失败，请稍后重试")
		return
	}
	next := int64(0)
	if len(messages) == limit && len(messages) > 0 {
		next = messages[len(messages)-1].ID
	}
	writeData(w, http.StatusOK, map[string]any{
		"items":      messages,
		"nextCursor": next,
	})
}

func (h *Handler) createMessage(w http.ResponseWriter, r *http.Request) {
	var req createMessageRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "请求体格式不正确")
		return
	}
	if !allowedMessageTypes[req.Type] {
		writeError(w, http.StatusBadRequest, "invalid_type", "消息类型只能是 praise、rant 或 thought")
		return
	}
	content, err := cleanText(req.Content, 2000, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_content", "内容需在 1-2000 字之间")
		return
	}
	nickname, err := cleanNickname(req.Nickname)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_nickname", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	msg, err := h.store.CreateMessage(ctx, content, req.Type, nickname)
	if err != nil {
		h.logger.Error("发布留言失败", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "发布失败，请稍后重试")
		return
	}
	writeData(w, http.StatusCreated, msg)
}

func (h *Handler) listReplies(w http.ResponseWriter, r *http.Request) {
	messageID, err := parseID(chi.URLParam(r, "id"), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "留言 ID 不合法")
		return
	}
	limit := clampInt(r.URL.Query().Get("limit"), 50, 1, 100)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	replies, err := h.store.ListReplies(ctx, messageID, limit)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "这条留言已经漂走了")
		return
	}
	if err != nil {
		h.logger.Error("查询回复失败", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "回复加载失败")
		return
	}
	writeData(w, http.StatusOK, map[string]any{"items": replies})
}

func (h *Handler) createReply(w http.ResponseWriter, r *http.Request) {
	messageID, err := parseID(chi.URLParam(r, "id"), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "留言 ID 不合法")
		return
	}
	var req createReplyRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "请求体格式不正确")
		return
	}
	content, err := cleanText(req.Content, 1000, true)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_content", "回复需在 1-1000 字之间")
		return
	}
	nickname, err := cleanNickname(req.Nickname)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_nickname", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	reply, err := h.store.CreateReply(ctx, messageID, content, nickname)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "这条留言已经漂走了")
		return
	}
	if err != nil {
		h.logger.Error("发布回复失败", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "回复失败，请稍后重试")
		return
	}
	writeData(w, http.StatusCreated, reply)
}

func (h *Handler) adminDeleteMessage(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "留言 ID 不合法")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.store.DeleteMessage(ctx, id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "留言不存在")
		return
	} else if err != nil {
		h.logger.Error("删除留言失败", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "删除失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) adminDeleteReply(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(chi.URLParam(r, "id"), false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "回复 ID 不合法")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.store.DeleteReply(ctx, id); errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "回复不存在")
		return
	} else if err != nil {
		h.logger.Error("删除回复失败", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "删除失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) adminPing(w http.ResponseWriter, r *http.Request) {
	writeData(w, http.StatusOK, map[string]any{"ok": true})
}
