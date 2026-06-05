// SPDX-License-Identifier: GPL-3.0-only

package render

import (
	"fmt"
	"strings"
)

// attrString pulls one kv value; errors yield .Error().
func attrString(args []any, key string) string {
	for i := 0; i+1 < len(args); i += 2 {
		k, ok := args[i].(string)
		if !ok || k != key {
			continue
		}
		v := args[i+1]
		if e, ok := v.(error); ok {
			return e.Error()
		}
		return fmt.Sprint(v)
	}
	return ""
}

// popMsg pulls a leading "msg" pair off args.
func popMsg(args []any) (string, []any) {
	if len(args) >= 2 {
		if k, ok := args[0].(string); ok && k == "msg" {
			if s, ok := args[1].(string); ok {
				return s, args[2:]
			}
		}
	}
	return "", args
}

func formatAttrs(args []any, colored bool) string {
	var b strings.Builder
	for i := 0; i+1 < len(args); i += 2 {
		k, ok := args[i].(string)
		if !ok {
			continue
		}
		v := args[i+1]
		if colored {
			_, _ = fmt.Fprintf(&b, " %s%s=%s%v", ansiCyan, k, ansiReset, v)
		} else {
			_, _ = fmt.Fprintf(&b, " %s=%v", k, v)
		}
	}
	return b.String()
}
