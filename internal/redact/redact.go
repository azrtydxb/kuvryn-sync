package redact

import (
	"regexp"
	"strings"
)

const marker = "REDACTED"

var assignments = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password|token|secret|credential|private[-_]?key|client[-_]?secret)(\s*[:=]\s*)[^\s,;]+`),
	regexp.MustCompile(`(?i)(Authorization:\s*Bearer\s+)[^\s,;]+`),
}

// String removes obvious secret material from status, events, logs, and CLI output.
func String(value string) string {
	out := value
	for _, re := range assignments {
		out = re.ReplaceAllString(out, `${1}${2}`+marker)
	}
	return out
}

// Value returns a fully redacted placeholder when a field is sensitive.
func Value(value string) string {
	if value == "" || strings.Contains(value, marker) {
		return value
	}
	return marker
}

// SensitivePath reports whether a Kubernetes field path can contain secret material.
func SensitivePath(path string) bool {
	lower := strings.ToLower(path)
	if lower == "data" || lower == "stringdata" || strings.HasPrefix(lower, "data.") || strings.HasPrefix(lower, "stringdata.") {
		return true
	}
	for _, token := range []string{"password", "token", "secret", "credential", "privatekey", "private_key", "clientsecret", "client_secret"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}
