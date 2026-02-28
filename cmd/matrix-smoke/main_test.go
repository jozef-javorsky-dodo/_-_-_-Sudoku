package main

import (
	"fmt"
	"testing"
	"time"
)

func TestMatrixSmoke(t *testing.T) {
	t.Helper()

	*flagFailFast = true
	*flagVerbose = false
	*flagTimeout = 5 * time.Second
	*flagPayload = 64 // KiB
	*flagQuick = testing.Short()

	all := combos(*flagQuick)
	seen := make(map[string]struct{}, len(all))
	dedup := make([]combo, 0, len(all))
	for _, c := range all {
		cc := c.canonical()
		k := cc.String()
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		dedup = append(dedup, cc)
	}
	all = dedup

	for _, tc := range all {
		tc := tc
		name := fmt.Sprintf(
			"dl=%t_hm=%t_mode=%s_mux=%s_root=%s_ascii=%s_tables=%s",
			tc.enablePureDownlink,
			tc.httpmaskEnabled,
			tc.httpmaskMode,
			tc.mux,
			func() string {
				if tc.pathRoot == "" {
					return "none"
				}
				return tc.pathRoot
			}(),
			tc.asciiMode,
			tc.tableSet,
		)
		t.Run(name, func(t *testing.T) {
			if err := runOne(tc); err != nil {
				t.Fatalf("matrix smoke failed: %v", err)
			}
		})
	}
}
