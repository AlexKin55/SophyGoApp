package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "" +
		"PLAIN=one\n" +
		"[common]\n" +
		"COMMON_KEY=two\n" +
		"[test]\n" +
		"KEY=three\n" +
		"NESTED=four\n"

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("не удалось записать .env: %v", err)
	}

	keys := []string{"PLAIN", "COMMON_KEY", "TEST_KEY", "TEST_NESTED"}
	for _, key := range keys {
		_ = os.Unsetenv(key)
	}
	t.Cleanup(func() {
		for _, key := range keys {
			_ = os.Unsetenv(key)
		}
	})

	if err := Load(path); err != nil {
		t.Fatalf("Load() вернул ошибку: %v", err)
	}

	expected := map[string]string{
		"PLAIN":       "one",
		"COMMON_KEY":  "two",
		"TEST_KEY":    "three",
		"TEST_NESTED": "four",
	}
	for key, want := range expected {
		if got := os.Getenv(key); got != want {
			t.Errorf("%s = %q, ожидали %q", key, got, want)
		}
	}
}

func TestSectionPrefix(t *testing.T) {
	cases := map[string]string{
		"":         "",
		"common":   "",
		"global":   "",
		"telegram": "TELEGRAM_",
		"MAX":      "MAX_",
		"my-api":   "MY_API_",
	}

	for name, want := range cases {
		if got := sectionPrefix(name); got != want {
			t.Errorf("sectionPrefix(%q) = %q, ожидали %q", name, got, want)
		}
	}
}
