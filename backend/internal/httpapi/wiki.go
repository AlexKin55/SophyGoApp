package httpapi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	wikiAPIURL   = "https://ru.wikipedia.org/w/api.php"
	maxWikiBody  = 256 << 10
	maxWikiReply = 1 << 20
	maxPhotoBody = 10 << 20
)

// photoHosts — домены, с которых разрешено проксировать фотографии.
var photoHosts = map[string]struct{}{
	"upload.wikimedia.org":  {},
	"commons.wikimedia.org": {},
	"ru.wikipedia.org":      {},
	"thumb.wikimedia.org":   {},
}

// handleWikiProxy проксирует запросы к Wikipedia API через backend.
// Нужен, когда прямой доступ к ru.wikipedia.org из WebView заблокирован.
func (s *Server) handleWikiProxy(w http.ResponseWriter, r *http.Request) {
	target := wikiAPIURL
	var body io.Reader

	if r.Method == http.MethodGet {
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
	} else {
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWikiBody))
		if err != nil {
			writeError(w, http.StatusRequestEntityTooLarge, "тело запроса слишком большое")
			return
		}
		body = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "некорректный запрос")
		return
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "SophyGo/1.0 (wiki proxy)")
	if r.Method != http.MethodGet {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.logger.Warn("wiki proxy request failed", "error", err)
		writeError(w, http.StatusBadGateway, "Wikipedia недоступна")
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, "Wikipedia вернула ошибку")
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, maxWikiReply))
}

// handlePhotoProxy загружает фотографию с Wikimedia через backend.
func (s *Server) handlePhotoProxy(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("url"))
	if raw == "" {
		writeError(w, http.StatusBadRequest, "url обязателен")
		return
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" {
		writeError(w, http.StatusBadRequest, "некорректный url")
		return
	}
	if _, ok := photoHosts[strings.ToLower(parsed.Hostname())]; !ok {
		writeError(w, http.StatusForbidden, "домен не разрешён")
		return
	}

	client := &http.Client{
		Timeout: 12 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("слишком много редиректов")
			}
			if _, ok := photoHosts[strings.ToLower(req.URL.Hostname())]; !ok {
				return fmt.Errorf("редирект на неразрешённый домен")
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, parsed.String(), nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, "некорректный url")
		return
	}
	req.Header.Set("User-Agent", "SophyGo/1.0 (photo proxy)")
	req.Header.Set("Accept", "image/*")

	resp, err := client.Do(req)
	if err != nil {
		s.logger.Warn("photo proxy request failed", "error", err, "host", parsed.Hostname())
		writeError(w, http.StatusBadGateway, "не удалось загрузить изображение")
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, "источник вернул ошибку")
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, maxPhotoBody))
}
