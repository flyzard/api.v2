package saft

import (
	"strings"
	"testing"
)

func TestTranscodeWin1252_ReportsOffendingRune(t *testing.T) {
	_, err := transcodeWin1252([]byte("<CompanyName>Café \U0001F600 Lda</CompanyName>"))
	if err == nil {
		t.Fatal("want error for unmappable rune")
	}
	msg := err.Error()
	if !strings.Contains(msg, "\U0001F600") || !strings.Contains(msg, "byte") {
		t.Fatalf("error lacks offender context: %v", err)
	}
	if !strings.Contains(msg, "CompanyName") {
		t.Fatalf("error lacks surrounding XML context: %v", err)
	}
}

func TestTranscodeWin1252_AcceptsLatin(t *testing.T) {
	out, err := transcodeWin1252([]byte("<x>Açores São João €</x>"))
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("empty output")
	}
}
