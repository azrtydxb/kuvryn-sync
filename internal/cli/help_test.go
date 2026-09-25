package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/azrtydxb/kuvryn-sync/internal/brand"
)

func TestKsyncVersionAndHelp(t *testing.T) {
	var out, errb bytes.Buffer
	if _, code := Run(context.Background(), []string{"version"}, &out, &errb); code != 0 {
		t.Fatalf("version exit %d: %s", code, errb.String())
	}
	if !strings.HasPrefix(out.String(), "ksync ") {
		t.Fatalf("version = %q, want ksync <version>", out.String())
	}
	out.Reset()
	Run(context.Background(), []string{"help"}, &out, &errb)
	if strings.Contains(out.String(), brand.OldName) || !strings.Contains(out.String(), "ksync diagnose") {
		t.Fatalf("help still names the old CLI:\n%s", out.String())
	}
}
