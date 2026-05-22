package domain

import (
	"encoding/json"
	"testing"
)

func TestPeriodValidate(t *testing.T) {
	tests := []struct {
		in      int
		wantErr bool
	}{
		{1, false},
		{12, false},
		{6, false},
		{0, true},
		{13, true},
		{-1, true},
		{16, true},
	}
	for _, tc := range tests {
		_, err := NewPeriod(tc.in)
		if (err != nil) != tc.wantErr {
			t.Errorf("NewPeriod(%d): err=%v want err=%v", tc.in, err, tc.wantErr)
		}
	}
}

func TestPeriodJSON(t *testing.T) {
	p, err := NewPeriod(7)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "7" {
		t.Fatalf("marshal: got %s want 7", b)
	}
	var got Period
	if err := json.Unmarshal([]byte("12"), &got); err != nil {
		t.Fatal(err)
	}
	if got != 12 {
		t.Fatalf("unmarshal: got %d want 12", got)
	}
	if err := json.Unmarshal([]byte("13"), &got); err == nil {
		t.Fatal("unmarshal 13: expected error, got nil")
	}
}
