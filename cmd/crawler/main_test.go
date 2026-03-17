package main

import (
	"reflect"
	"testing"
)

func TestCommentFetchCappedWarningIncludesRetrievedCount(t *testing.T) {
	got := commentFetchCappedWarning(37, 40, 10, 4)
	want := "comment fetch capped: retrieved 37 comments; not all comments were retrieved (limits: top=40, child=10, depth=4); comments critique may be incomplete"
	if got != want {
		t.Fatalf("warning mismatch:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestParseStoryIDs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []int
		wantErr bool
	}{
		{
			name:  "accepts semicolon separated HN URLs",
			input: "https://news.ycombinator.com/item?id=123;https://news.ycombinator.com/item?id=456",
			want:  []int{123, 456},
		},
		{
			name:  "accepts IDs and ignores empty segments",
			input: "123;; 456 ;",
			want:  []int{123, 456},
		},
		{
			name:    "fails when URL missing id",
			input:   "https://news.ycombinator.com/item",
			wantErr: true,
		},
		{
			name:    "fails when no valid values are present",
			input:   " ; ; ",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseStoryIDs(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(tc.want, got) {
				t.Fatalf("parseStoryIDs mismatch: want %v, got %v", tc.want, got)
			}
		})
	}
}
