package redact

import "testing"

func TestStringRedactsAssignments(t *testing.T) {
	got := String("render failed password=super-secret token:abc Authorization: Bearer xyz")
	for _, leak := range []string{"super-secret", "abc", "xyz"} {
		if contains(got, leak) {
			t.Fatalf("redaction leaked %q in %q", leak, got)
		}
	}
}

func TestStringRedactsURLUserinfo(t *testing.T) {
	credentials := "deploy:" + "hunter" + "2"
	cases := map[string]string{
		"pull https://" + credentials + "@registry.example/v2/api failed": "pull https://REDACTED@registry.example/v2/api failed",
		"clone git+ssh://" + credentials + "@git.example/repo.git":        "clone git+ssh://REDACTED@git.example/repo.git",
		"token-only https://abc123@example.com/x":                         "token-only https://REDACTED@example.com/x",
		"no credentials in https://example.com/a@b":                       "no credentials in https://example.com/a@b",
		"mail user@example.com stays":                                     "mail user@example.com stays",
	}
	for in, want := range cases {
		if got := String(in); got != want {
			t.Errorf("String(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSensitivePath(t *testing.T) {
	for _, path := range []string{"data", "stringData", "spec.template.env.password", "token"} {
		if !SensitivePath(path) {
			t.Fatalf("path %q not sensitive", path)
		}
	}
	if SensitivePath("metadata.name") {
		t.Fatal("metadata.name should not be sensitive")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
