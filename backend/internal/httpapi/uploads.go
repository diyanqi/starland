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

	"shiguang-island/backend/internal/media"
)

var uploadNamePattern = regexp.MustCompile(`^[a-z0-9]+-[a-f0-9]+\.(jpg|jpeg|png|gif|webp|avif|mp4|mov|webm)$`)

var uploadContentTypes = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".gif":  "image/gif",
	".webp": "image/webp",
	".avif": "image/avif",
	".mp4":  "video/mp4",
	".mov":  "video/quicktime",
	".webm": "video/webm",
}

var errUploadTooLarge = errors.New("上传文件超过大小限制")

func (h *Handler) createPhoto(w http.ResponseWriter, r *http.Request) {
	// multipart 表单本身有额外开销，留 1MB 余量。视频上限通常高于图片上限。
	bodyLimit := max(h.cfg.MaxUploadBytes, h.cfg.MaxVideoBytes) + (1 << 20)
	r.Body = http.MaxBytesReader(w, r.Body, bodyLimit)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "文件太大了，请压缩后再上传")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_multipart", "上传表单格式不正确")
		return
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()

	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing_image", "请选择一张图片或实况视频")
		return
	}
	defer file.Close()

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

	prefix := make([]byte, 4096)
	n, readErr := io.ReadFull(file, prefix)
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		writeError(w, http.StatusBadRequest, "read_failed", "文件读取失败")
		return
	}
	if n == 0 {
		writeError(w, http.StatusBadRequest, "empty_file", "文件内容为空")
		return
	}

	format, ok := media.Detect(prefix[:n])
	if !ok {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_type",
			"暂不支持这个文件格式，请上传 JPG / PNG / GIF / WebP / HEIC / AVIF 图片，或 MP4 / MOV / WebM 视频")
		return
	}

	limit := h.cfg.MaxUploadBytes
	if format.Kind == media.KindVideo {
		limit = h.cfg.MaxVideoBytes
	}
	if header.Size > limit {
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", tooLargeMessage(format.Kind))
		return
	}

	src := io.MultiReader(bytes.NewReader(prefix[:n]), file)
	var name string
	if format.Transcode {
		data, err := io.ReadAll(io.LimitReader(src, limit+1))
		if err != nil {
			writeError(w, http.StatusBadRequest, "read_failed", "文件读取失败")
			return
		}
		if int64(len(data)) > limit {
			writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", tooLargeMessage(format.Kind))
			return
		}
		jpegData, err := media.TranscodeToJPEG(data, format.Ext)
		if err != nil {
			h.logger.Warn("图片转码失败", "error", err, "format", format.Ext, "size", len(data))
			if errors.Is(err, media.ErrTooLarge) {
				writeError(w, http.StatusBadRequest, "image_too_large", "图片分辨率太高，请压缩后再上传")
				return
			}
			writeError(w, http.StatusBadRequest, "invalid_image", "这张图片无法解析，请换一张试试")
			return
		}
		name, err = writeUpload(h.cfg.UploadDir, ".jpg", bytes.NewReader(jpegData), limit)
		if err != nil {
			h.handleUploadWriteError(w, err)
			return
		}
	} else {
		name, err = writeUpload(h.cfg.UploadDir, format.Ext, src, limit)
		if err != nil {
			h.handleUploadWriteError(w, err)
			return
		}
	}

	url := "/uploads/" + name
	ctx := r.Context()
	photo, err := h.store.CreatePhoto(ctx, url, caption, nickname, string(format.Kind))
	if err != nil {
		_ = os.Remove(filepath.Join(h.cfg.UploadDir, name))
		h.logger.Error("写入照片记录失败", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "照片保存失败，请稍后重试")
		return
	}
	writeData(w, http.StatusCreated, photo)
}

func (h *Handler) handleUploadWriteError(w http.ResponseWriter, err error) {
	if errors.Is(err, errUploadTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "file_too_large", "文件太大了，请压缩后再上传")
		return
	}
	h.logger.Error("保存上传文件失败", "error", err)
	writeError(w, http.StatusInternalServerError, "storage_error", "文件保存失败")
}

func tooLargeMessage(kind media.Kind) string {
	if kind == media.KindVideo {
		return "视频太大了，请压缩后再上传"
	}
	return "图片太大了，请压缩后再上传"
}

func writeUpload(dir, ext string, src io.Reader, limit int64) (string, error) {
	name := fmt.Sprintf("%s-%s%s", time.Now().UTC().Format("20060102"), randomHex(8), ext)
	dstPath := filepath.Join(dir, name)
	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}

	written, copyErr := io.Copy(dst, io.LimitReader(src, limit+1))
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(dstPath)
		return "", errors.Join(copyErr, closeErr)
	}
	if written > limit {
		_ = os.Remove(dstPath)
		return "", errUploadTooLarge
	}
	return name, nil
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

	ext := strings.ToLower(filepath.Ext(name))
	if ct := uploadContentTypes[ext]; ct != "" {
		w.Header().Set("Content-Type", ct)
	} else if ct := mime.TypeByExtension(ext); ct != "" {
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

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
