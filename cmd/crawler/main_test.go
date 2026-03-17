package main

import "testing"

func TestCommentFetchCappedWarningIncludesRetrievedCount(t *testing.T) {
	got := commentFetchCappedWarning(37, 40, 10, 4)
	want := "comment fetch capped: retrieved 37 comments; not all comments were retrieved (limits: top=40, child=10, depth=4); comments critique may be incomplete"
	if got != want {
		t.Fatalf("warning mismatch:\nwant: %q\ngot:  %q", want, got)
	}
}
