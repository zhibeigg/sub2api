package qqbot

import "testing"

func TestParseCommandAndEmailHelpers(t *testing.T) {
	tests := []struct {
		raw   string
		kind  CommandKind
		email string
	}{
		{"<@!bot> /bind User@Example.com", CommandBind, "user@example.com"},
		{"绑定 user@example.com", CommandBind, "user@example.com"},
		{"/帮助", CommandHelp, ""},
		{"hello", CommandNone, ""},
	}
	for _, test := range tests {
		got := ParseCommand(test.raw)
		if got.Kind != test.kind || got.Email != test.email {
			t.Fatalf("ParseCommand(%q) = %#v", test.raw, got)
		}
	}
	if !ValidEmail("user@example.com") || ValidEmail("bad@example") {
		t.Fatal("email validation mismatch")
	}
	if got := MaskEmail("user@example.com"); got != "u***r@example.com" {
		t.Fatalf("mask = %q", got)
	}
}
