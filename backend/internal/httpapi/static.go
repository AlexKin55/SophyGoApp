package httpapi

import (
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Регистрируем типы, которые нужны VAD/ONNX Runtime, но не всегда есть
// в системной таблице MIME. Без них браузер откажется исполнять .wasm.
func init() {
	_ = mime.AddExtensionType(".wasm", "application/wasm")
	_ = mime.AddExtensionType(".mjs", "text/javascript")
	_ = mime.AddExtensionType(".js", "text/javascript")
}

// handleNotFound отдаёт собранный frontend для SPA-маршрутов и 404 для API.
func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	if s.static == nil || strings.HasPrefix(r.URL.Path, "/api") {
		http.NotFound(w, r)
		return
	}
	s.static.ServeHTTP(w, r)
}

// staticHandler раздаёт файлы из каталога сборки frontend с SPA-фолбэком:
// любой маршрут без расширения возвращает index.html.
type staticHandler struct {
	dir string
}

func newStaticHandler(dir string) http.Handler {
	return &staticHandler{dir: filepath.Clean(dir)}
}

func (h *staticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	urlPath := path.Clean("/" + r.URL.Path)
	filePath := filepath.Join(h.dir, filepath.FromSlash(urlPath))

	// Защита от выхода за пределы каталога сборки.
	if filePath != h.dir && !strings.HasPrefix(filePath, h.dir+string(os.PathSeparator)) {
		http.NotFound(w, r)
		return
	}

	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		h.serveFile(w, r, filePath)
		return
	}

	// SPA-фолбэк: маршруты приложения не имеют расширения.
	if path.Ext(urlPath) == "" {
		h.serveFile(w, r, filepath.Join(h.dir, "index.html"))
		return
	}

	http.NotFound(w, r)
}

func (h *staticHandler) serveFile(w http.ResponseWriter, r *http.Request, filePath string) {
	switch {
	case strings.HasSuffix(filePath, ".html"):
		// index.html не кэшируем: после деплоя MAX WebView должен
		// подхватить новую ссылку на JS-бандл.
		w.Header().Set("Cache-Control", "no-store, max-age=0")
	case strings.Contains(filePath, string(os.PathSeparator)+"assets"+string(os.PathSeparator)):
		// Имена файлов в assets содержат хеш, поэтому кэшируем надолго.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	default:
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}
	http.ServeFile(w, r, filePath)
}
