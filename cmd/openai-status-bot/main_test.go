package main

import "testing"

func TestFrameworkInitialOffsetSeedsLastSeenUpdate(t *testing.T) {
	tests := []struct {
		name   string
		stored int64
		want   int64
		ok     bool
	}{
		{name: "empty", stored: 0, ok: false},
		{name: "invalid negative", stored: -1, ok: false},
		{name: "next update", stored: 42, want: 41, ok: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := frameworkInitialOffset(tt.stored)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("frameworkInitialOffset(%d) = (%d, %v), want (%d, %v)", tt.stored, got, ok, tt.want, tt.ok)
			}
		})
	}
}
