package httpapi

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var uploadNamePattern = regexp.MustCompile(`^[a-z0-9]+-[a-f0-9]+\.(jpg|jpeg|png|gif|webp)$`)

func (h *Handler) createPhoto(w http.ResponseWriter, r *http.Request) {
	// multipart 表单本身有额外开销，留 1MB 余量。
	r.Body = http.MaxBytesReader(w, r.Body, h.cfg.MaxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "图片太大了，请压缩后再上传")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_multipart", "上传表单格式不正确")
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_image", "请选择一张图片")
		return
	}
	defer file.Close()

	if header.Size > h.cfg.MaxUploadBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "图片太大了，请压缩后再上传")
		return
	}

	caption, err := cleanText(r.FormValue("caption"), 200, false)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_caption", "图注最多 200 个字")
		return
	}
	nickname, err := cleanNickname(r.FormValue("nickname"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_nickname", err.Error())
		return
	}

	prefix := make([]byte, 512)
	n, readErr := io.ReadFull(file, prefix)
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		writeError(w, http.StatusBadRequest, "read_failed", "图片读取失败")
		return
	}
	if n == 0 {
		writeError(w, http.StatusBadRequest, "empty_file", "图片内容为空")
		return
	}
	ext, ok := detectImageExt(prefix[:n])
	if !ok {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_type", "仅支持 JPG、PNG、GIF、WebP 图片")
		return
	}

	name := fmt.Sprintf("%s-%s%s", time.Now().UTC().Format("20060102"), randomHex(8), ext)
	dstPath := filepath.Join(h.cfg.UploadDir, name)
	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		h.logger.Error("创建图片文件失败", "error", err)
		writeError(w, http.StatusInternalServerError, "storage_error", "图片保存失败")
		return
	}

	src := io.MultiReader(bytes.NewReader(prefix[:n]), file)
	written, copyErr := io.Copy(dst, io.LimitReader(src, h.cfg.MaxUploadBytes+1))
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(dstPath)
		h.logger.Error("写入图片失败", "error", errors.Join(copyErr, closeErr))
		writeError(w, http.StatusInternalServerError, "storage_error", "图片保存失败")
		return
	}
	if written > h.cfg.MaxUploadBytes {
		_ = os.Remove(dstPath)
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "图片太大了，请压缩后再上传")
		return
	}

	url := "/uploads/" + name
	ctx := r.Context()
	photo, err := h.store.CreatePhoto(ctx, url, caption, nickname)
	if err != nil {
		_ = os.Remove(dstPath)
		h.logger.Error("写入照片记录失败", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "照片保存失败，请稍后重试")
		return
	}
	writeData(w, http.StatusCreated, photo)
}

func (h *Handler) serveUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不被允许")
		return
	}
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/uploads/")
	if !uploadNamePattern.MatchString(name) {
		http.NotFound(w, r)
		return
	}

	f, err := os.Open(filepath.Join(h.cfg.UploadDir, name))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	if ct := mime.TypeByExtension(filepath.Ext(name)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, name, info.ModTime(), f)
}

// removeUploadedFile 仅删除本服务生成的 /uploads/ 文件，忽略外部 URL。
func (h *Handler) removeUploadedFile(url string) {
	if !strings.HasPrefix(url, "/uploads/") {
		return
	}
	name := strings.TrimPrefix(url, "/uploads/")
	if !uploadNamePattern.MatchString(name) {
		return
	}
	if err := os.Remove(filepath.Join(h.cfg.UploadDir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		h.logger.Warn("删除图片文件失败", "error", err, "file", name)
	}
}

func detectImageExt(prefix []byte) (string, bool) {
	switch http.DetectContentType(prefix) {
	case "image/jpeg":
		return ".jpg", true
	case "image/png":
		return ".png", true
	case "image/gif":
		return ".gif", true
	case "image/webp":
		return ".webp", true
	}
	if len(prefix) >= 12 && string(prefix[0:4]) == "RIFF" && string(prefix[8:12]) == "WEBP" {
		return ".webp", true
	}
	return "", false
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
