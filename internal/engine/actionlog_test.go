// SPDX-License-Identifier: GPL-3.0-only

package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Error values handed to Emit must serialize as their Error() string,
// not the default JSON struct encoding of {}.
func TestActionLog_RecordsErrorAsString(t *testing.T) {
	dir := t.TempDir()
	al, err := NewActionLog(dir)
	if err != nil {
		t.Fatal(err)
	}
	al.Emit(context.Background(), CodeSnapshotRejected, nil,
		"phase", "typecheck",
		"err", errors.New(`file.x: missing required attr "content"`),
	)
	if err := al.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "0001.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if strings.Contains(s, `"err":{}`) {
		t.Errorf("err serialized as empty object; got %q", s)
	}
	if !strings.Contains(s, "missing required attr") {
		t.Errorf("err message missing; got %q", s)
	}
}
