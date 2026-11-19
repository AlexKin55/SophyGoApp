package events

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store дописывает события в JSONL-файл и держит последние из них в памяти.
//
// JSONL выбран вместо базы данных на текущем этапе: он не требует
// инфраструктуры, переживает перезапуск процесса и легко читается
// стандартными инструментами. При переходе на PostgreSQL реализация
// заменяется, интерфейс использования остаётся прежним.
type Store struct {
	mu        sync.Mutex
	file      *os.File
	encoder   *json.Encoder
	recent    []Event
	maxRecent int
}

// Open создаёт хранилище: открывает (или создаёт) файл и готовит буфер в памяти.
func Open(path string, maxRecent int) (*Store, error) {
	if maxRecent <= 0 {
		return nil, fmt.Errorf("maxRecent должен быть положительным, получено %d", maxRecent)
	}

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("создание каталога %q: %w", dir, err)
		}
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("открытие файла событий %q: %w", path, err)
	}

	return &Store{
		file:      file,
		encoder:   json.NewEncoder(file),
		recent:    make([]Event, 0, maxRecent),
		maxRecent: maxRecent,
	}, nil
}

// Append сохраняет событие: сначала в файл, затем в буфер последних событий.
func (s *Store) Append(ctx context.Context, event Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.encoder.Encode(event); err != nil {
		return fmt.Errorf("запись события: %w", err)
	}

	if len(s.recent) < s.maxRecent {
		s.recent = append(s.recent, event)
	} else {
		copy(s.recent, s.recent[1:])
		s.recent[len(s.recent)-1] = event
	}

	return nil
}

// Recent возвращает последние события в порядке от новых к старым.
func (s *Store) Recent(limit int) []Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 || limit > len(s.recent) {
		limit = len(s.recent)
	}

	result := make([]Event, limit)
	for i := 0; i < limit; i++ {
		result[i] = s.recent[len(s.recent)-1-i]
	}
	return result
}

// Close закрывает файл событий.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	return err
}
