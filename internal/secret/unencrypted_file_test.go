// SPDX-License-Identifier: GPL-3.0-only

package secret

import "testing"

func TestFileBackend_Lookup(t *testing.T) {
	data := []byte(`{"db_pass": "hunter2", "api_token": "tok-abc"}`)

	b, err := NewFileBackend(data)
	if err != nil {
		t.Fatalf("NewFileBackend: %v", err)
	}

	if b.Name() != "unencrypted_file" {
		t.Errorf("Name: got %q, want %q", b.Name(), "unencrypted_file")
	}

	val, ok, err := b.Lookup("db_pass")
	if err != nil {
		t.Fatalf("Lookup(db_pass): %v", err)
	}
	if !ok {
		t.Fatal("Lookup(db_pass): not found")
	}
	if val != "hunter2" {
		t.Errorf("Lookup(db_pass): got %q, want %q", val, "hunter2")
	}

	_, ok, err = b.Lookup("missing")
	if err != nil {
		t.Fatalf("Lookup(missing): %v", err)
	}
	if ok {
		t.Error("Lookup(missing): should not be found")
	}
}

func TestFileBackend_InvalidJSON(t *testing.T) {
	_, err := NewFileBackend([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestFileBackend_Empty(t *testing.T) {
	b, err := NewFileBackend([]byte(`{}`))
	if err != nil {
		t.Fatalf("NewFileBackend: %v", err)
	}

	_, ok, err := b.Lookup("anything")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if ok {
		t.Error("Lookup: should not find anything in empty backend")
	}
}
