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
