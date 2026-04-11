// SPDX-License-Identifier: GPL-3.0-only

package fileops

import (
	"context"
	"errors"
	"testing"
)

func TestVerifiedWrite_PlaceholderCount(t *testing.T) {
	tests := []struct {
		name      string
		verifyCmd string
		wantCount int
	}{
		{
			name:      "no placeholder",
			verifyCmd: "true",
			wantCount: 0,
		},
		{
			name:      "two placeholders",
			verifyCmd: "diff %s %s",
			wantCount: 2,
		},
		{
			name:      "three placeholders interleaved",
			verifyCmd: "cat %s | grep %s | tee %s",
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// nil target is fine: the placeholder check fires before
			// any target method is invoked, so the test exercises only
			// the validation path.
			err := VerifiedWrite(context.Background(), nil, "/etc/foo.conf", []byte("x"), tt.verifyCmd)
			var pe *VerifyPlaceholderError
			if !errors.As(err, &pe) {
				t.Fatalf("got %T %v, want *VerifyPlaceholderError", err, err)
			}
			if pe.Count != tt.wantCount {
				t.Errorf("Count = %d, want %d", pe.Count, tt.wantCount)
			}
			if pe.Cmd != tt.verifyCmd {
				t.Errorf("Cmd = %q, want %q", pe.Cmd, tt.verifyCmd)
			}
		})
	}
}
