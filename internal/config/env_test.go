package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv_QuoteStripping(t *testing.T) {
	cases := map[string]string{
		`PLAIN=abc`:      "abc",
		`DQ="abc"`:       "abc",
		`SQ='abc'`:       "abc",
		`TRAILQ="abc'"`:  "abc'",  // inner quote is part of the password
		`NESTED=''x''`:   "'x'",   // strip ONE pair only
		`MISMATCH="abc'`: `"abc'`, // no matching pair — leave intact
	}
	content := ""
	for line := range cases {
		content += line + "\n"
	}
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"PLAIN", "DQ", "SQ", "TRAILQ", "NESTED", "MISMATCH"} {
		os.Unsetenv(k)
	}
	if err := loadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"PLAIN": "abc", "DQ": "abc", "SQ": "abc",
		"TRAILQ": "abc'", "NESTED": "'x'", "MISMATCH": `"abc'`,
	}
	for k, w := range want {
		if got := os.Getenv(k); got != w {
			t.Errorf("%s = %q, want %q", k, got, w)
		}
		os.Unsetenv(k)
	}
}
