package brand

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// history lists the only places the old name may remain: sections that tell
// the product's history, keyed by file and identified by their heading.
var history = map[string]string{
	"kuvryn-sync-full-spec.md": "## History",
	"docs/upgrade.md":          "Moving from Solder 0.3.x to Kuvryn Sync 0.4.0",
}

// TestNoSolderNameRemains fails while the old product name is left anywhere in
// the repository outside its history: CHANGELOG.md, .procoder/, go.sum, and
// the sections listed in history. This package names it on purpose.
func TestNoSolderNameRemains(t *testing.T) {
	root := filepath.Join("..", "..")
	out, err := exec.Command("git", "-C", root, "grep", "--untracked", "-n", "-i", OldName, "--", ".",
		":!.procoder", ":!CHANGELOG.md", ":!go.sum", ":!internal/brand").Output()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return // no match at all
	}
	if err != nil {
		t.Fatalf("git grep failed: %v", err)
	}
	var left []string
	for _, match := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		file, rest, _ := strings.Cut(match, ":")
		number, _, _ := strings.Cut(rest, ":")
		line, _ := strconv.Atoi(number)
		if heading, ok := history[file]; ok {
			if span := section(t, filepath.Join(root, file), heading); line >= span[0] && line < span[1] {
				continue
			}
		}
		left = append(left, match)
	}
	if len(left) > 0 {
		t.Fatalf("the old product name remains outside its history (%d lines):\n%s", len(left), strings.Join(left, "\n"))
	}
}

// section returns the first line and the line after the last of the Markdown
// section whose heading contains heading: up to the next heading of the same
// or a higher level, outside code fences. Without that heading the span is
// empty, so every line of the file is checked.
func section(t *testing.T, path, heading string) [2]int {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("history file: %v", err)
	}
	defer func() { _ = file.Close() }()
	start, level, number, fenced := 0, 0, 0, false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		number++
		text := scanner.Text()
		if strings.HasPrefix(text, "```") {
			fenced = !fenced
		}
		if fenced || !strings.HasPrefix(text, "#") {
			continue
		}
		hashes := len(text) - len(strings.TrimLeft(text, "#"))
		if start == 0 && strings.Contains(text, heading) {
			start, level = number, hashes
			continue
		}
		if start != 0 && hashes <= level {
			return [2]int{start, number}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if start == 0 {
		return [2]int{}
	}
	return [2]int{start, number + 1}
}
