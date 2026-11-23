// Package envfile загружает переменные окружения из файла .env.
//
// Формат совместим с shell: строки KEY=VALUE, комментарии с #,
// значения с пробелами можно заключать в одинарные или двойные кавычки.
// Уже заданные в окружении переменные не перезаписываются.
//
// Поддерживаются секции: ключи внутри [telegram] получают префикс
// TELEGRAM_, внутри [max] — MAX_. Секция [common] (и ключи вне секций)
// оставляет имена без изменений. Это позволяет держать настройки
// нескольких мессенджеров в одном файле:
//
//	[common]
//	HTTP_ADDR=:8080
//
//	[telegram]
//	BOT_TOKEN=...
//
//	[max]
//	BOT_TOKEN=...
package envfile

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Load читает файл и выставляет отсутствующие в окружении переменные.
func Load(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	prefix := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			prefix = sectionPrefix(line[1 : len(line)-1])
			continue
		}

		line = strings.TrimPrefix(line, "export ")

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		fullKey := prefix + key
		if _, exists := os.LookupEnv(fullKey); exists {
			continue
		}
		if err := os.Setenv(fullKey, trimQuotes(strings.TrimSpace(value))); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// sectionPrefix превращает имя секции в префикс переменных.
// Секция [common] не добавляет префикс.
func sectionPrefix(name string) string {
	normalized := strings.TrimSpace(strings.ToUpper(name))
	if normalized == "" || normalized == "COMMON" || normalized == "GLOBAL" {
		return ""
	}

	normalized = strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, normalized)

	return normalized + "_"
}

// LoadNearest ищет файл с указанным именем в текущей директории и выше.
// Возвращает путь к найденному файлу. Если файл не найден — os.ErrNotExist.
func LoadNearest(name string) (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for depth := 0; depth < 6; depth++ {
		candidate := filepath.Join(dir, name)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, Load(candidate)
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", os.ErrNotExist
}

func trimQuotes(value string) string {
	if len(value) < 2 {
		return value
	}
	first, last := value[0], value[len(value)-1]
	if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}
