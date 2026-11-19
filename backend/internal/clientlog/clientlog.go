// Package clientlog принимает логи frontend и пишет их в JSONL-файл.
package clientlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Entry — одна запись, присланная браузером.
type Entry struct {
	Timestamp string          `json:"timestamp,omitempty"`
	Level     string          `json:"level"`
	Message   string          `json:"message"`
	Context   json.RawMessage `json:"context,omitempty"`
	URL       string          `json:"url,omitempty"`
}

// Record — запись в файле: данные браузера плюс серверные поля.
type Record struct {
	ReceivedAt time.Time `json:"receivedAt"`
	RemoteIP   string    `json:"remoteIp,omitempty"`
	Entry
}

// Writer дописывает логи frontend в файл.
type Writer struct {
	mu      sync.Mutex
	file    *os.File
	encoder *json.Encoder
}

// Open создаёт writer. Пустой путь означает, что логирование отключено.
func Open(path string) (*Writer, error) {
	if path == "" {
		return nil, nil
	}

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("clientlog: создание каталога %q: %w", dir, err)
		}
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("clientlog: открытие файла %q: %w", path, err)
	}

	return &Writer{file: file, encoder: json.NewEncoder(file)}, nil
}

// Write добавляет записи в файл. Для nil-writer ничего не делает.
func (w *Writer) Write(remoteIP string, entries []Entry) error {
	if w == nil || len(entries) == 0 {
		return nil
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now().UTC()
	for _, entry := range entries {
		record := Record{ReceivedAt: now, RemoteIP: remoteIP, Entry: entry}
		if err := w.encoder.Encode(record); err != nil {
			return fmt.Errorf("clientlog: запись: %w", err)
		}
	}
	return nil
}

// Close закрывает файл.
func (w *Writer) Close() error {
	if w == nil {
		return nil
	}
	return w.file.Close()
}
