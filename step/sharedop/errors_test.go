// SPDX-License-Identifier: GPL-3.0-only

package sharedop

import (
	"errors"
	"fmt"
	"testing"

	"scampi.dev/scampi/diagnostic"
	"scampi.dev/scampi/target"
)

func TestDiagnoseTargetError_EscalationFailed(t *testing.T) {
	orig := target.EscalationError{
		Tool: "sudo", Op: "chmod", Path: "/etc/foo",
		Stderr: "not permitted", ExitCode: 1,
	}

	wrapped := DiagnoseTargetError(orig)

	var d diagnostic.Diagnostic
	if !errors.As(wrapped, &d) {
		t.Fatalf("expected diagnostic, got %T", wrapped)
	}
	if d.Impact() != diagnostic.ImpactAbort {
		t.Fatalf("expected ImpactAbort, got %v", d.Impact())
	}

	var efe EscalationFailedError
	if !errors.As(wrapped, &efe) {
		t.Fatalf("expected EscalationFailedError, got %T", wrapped)
	}
	if efe.Tool != "sudo" {
		t.Fatalf("expected tool %q, got %q", "sudo", efe.Tool)
	}
}

func TestDiagnoseTargetError_EscalationMissing(t *testing.T) {
	orig := target.NoEscalationError{Op: "apk install"}

	wrapped := DiagnoseTargetError(orig)

	var d diagnostic.Diagnostic
	if !errors.As(wrapped, &d) {
		t.Fatalf("expected diagnostic, got %T", wrapped)
	}

	var eme EscalationMissingError
	if !errors.As(wrapped, &eme) {
		t.Fatalf("expected EscalationMissingError, got %T", wrapped)
	}
	if eme.Op != "apk install" {
		t.Fatalf("expected op %q, got %q", "apk install", eme.Op)
	}
}

func TestDiagnoseTargetError_StagingError(t *testing.T) {
	orig := target.StagingError{
		Path: "/etc/config",
		Err:  fmt.Errorf("disk full"),
	}

	wrapped := DiagnoseTargetError(orig)

	var d diagnostic.Diagnostic
	if !errors.As(wrapped, &d) {
		t.Fatalf("expected diagnostic, got %T", wrapped)
	}

	var sfe StagingFailedError
	if !errors.As(wrapped, &sfe) {
		t.Fatalf("expected StagingFailedError, got %T", wrapped)
	}
	if sfe.Path != "/etc/config" {
		t.Fatalf("expected path %q, got %q", "/etc/config", sfe.Path)
	}
}

func TestDiagnoseTargetError_PassthroughUnknown(t *testing.T) {
	orig := fmt.Errorf("some other error")

	wrapped := DiagnoseTargetError(orig)

	if wrapped != orig {
		t.Fatalf("expected passthrough, got %T", wrapped)
	}
}

func TestEscalationErrors_StableEventIDs(t *testing.T) {
	missing := EscalationMissingError{NoEscalationError: target.NoEscalationError{Op: "chmod", Path: "/etc/foo"}}
	failed := EscalationFailedError{EscalationError: target.EscalationError{
		Tool: "sudo", Op: "chmod", Path: "/etc/foo", ExitCode: 1,
	}}

	mTmpl := missing.EventTemplate()
	fTmpl := failed.EventTemplate()

	if mTmpl.ID != "target.EscalationMissing" {
		t.Fatalf("expected ID %q, got %q", "target.EscalationMissing", mTmpl.ID)
	}
	if fTmpl.ID != "target.EscalationFailed" {
		t.Fatalf("expected ID %q, got %q", "target.EscalationFailed", fTmpl.ID)
	}
	if mTmpl.Hint == fTmpl.Hint {
		t.Fatal("expected different hints for missing vs failed escalation")
	}
}
