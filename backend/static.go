package main

import (
	"bytes"
	"embed"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// 前端构建产物会在编译期被嵌入二进制，部署时只需要一个可执行文件。
//
//go:embed all:web/dist
var frontendFS embed.FS

type staticHandler struct {
	fsys fs.FS
}

func newStaticHandler() (http.Handler, error) {
	dist, err := fs.Sub(frontendFS, "web/dist")
	if err != nil {
		return nil, err
	}
	return &staticHandler{fsys: dist}, nil
}

func (s *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clean := path.Clean("/" + r.URL.Path)
	rel := strings.TrimPrefix(clean, "/")
	if rel == "" {
		s.serveFile(w, r, "index.html", http.StatusOK)
		return
	}

	// Astro 默认输出 directory 格式（photos/index.html），同时兼容 photos.html。
	for _, candidate := range []string{rel, rel + "/index.html", rel + ".html"} {
		if s.serveFile(w, r, candidate, http.StatusOK) {
			return
		}
	}

	// SPA 式兜底：无扩展名且浏览器要 HTML 时返回首页，避免刷新出现 404。
	// 如果你更希望未知路径返回真正的 404 页面，删除这个分支即可。
	if !strings.Contains(path.Base(rel), ".") && strings.Contains(r.Header.Get("Accept"), "text/html") {
		if s.serveFile(w, r, "index.html", http.StatusOK) {
			return
		}
	}

	if !s.serveFile(w, r, "404.html", http.StatusNotFound) {
		http.NotFound(w, r)
	}
}

func (s *staticHandler) serveFile(w http.ResponseWriter, r *http.Request, name string, status int) bool {
	f, err := s.fsys.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		return false
	}

	ct := mime.TypeByExtension(path.Ext(name))
	if ct == "" {
		ct = "application/octet-stream"
	}
	lowerCT := strings.ToLower(ct)
	if (strings.HasPrefix(lowerCT, "text/") || lowerCT == "application/javascript" || lowerCT == "application/json") &&
		!strings.Contains(lowerCT, "charset=") {
		ct += "; charset=utf-8"
	}
	w.Header().Set("Content-Type", ct)
	if strings.HasPrefix(name, "_astro/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else if strings.HasSuffix(name, ".html") {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}

	if status != http.StatusOK {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
		w.WriteHeader(status)
		_, _ = io.Copy(w, f)
		return true
	}

	rs, ok := f.(io.ReadSeeker)
	if !ok {
		data, err := io.ReadAll(f)
		if err != nil {
			return false
		}
		http.ServeContent(w, r, name, info.ModTime(), bytes.NewReader(data))
		return true
	}
	http.ServeContent(w, r, name, info.ModTime(), rs)
	return true
}
