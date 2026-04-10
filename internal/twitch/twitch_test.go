package twitch

import (
	"os"
	"testing"
)

func TestLoadAndMatchCategories(t *testing.T) {
	// Point at the sample file in the repo root
	path := "../../categories-sample.yaml"
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("categories file not found at %s: %v", path, err)
	}
	os.Setenv("TW_CATEGORIES_FILE", path)
	defer os.Unsetenv("TW_CATEGORIES_FILE")

	cats, err := LoadCategories()
	if err != nil {
		t.Fatalf("LoadCategories error: %v", err)
	}
	if len(cats) == 0 {
		t.Fatal("LoadCategories returned empty slice")
	}
	t.Logf("loaded %d categories", len(cats))

	cases := []struct {
		topic   string
		want    string
		wantOK  bool
	}{
		{"☕️ taking a break", "Just Chatting", true},
		{"coding in go today", "Software and Game Development", true},
		{"yoga session", "Fitness & Health", true},
		{"random unmatched topic xyz", "", false},
	}

	for _, tc := range cases {
		got, ok := MatchCategory(tc.topic, cats)
		if ok != tc.wantOK {
			t.Errorf("MatchCategory(%q) ok=%v, want %v", tc.topic, ok, tc.wantOK)
			continue
		}
		if ok && got.Name != tc.want {
			t.Errorf("MatchCategory(%q) = %q, want %q", tc.topic, got.Name, tc.want)
		} else if ok {
			t.Logf("MatchCategory(%q) => %q ✓", tc.topic, got.Name)
		}
	}
}
