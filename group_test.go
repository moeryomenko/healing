package healing

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCheckGroup_Check(t *testing.T) {
	testcases := []struct {
		name          string
		checkers      []checkFunc
		checkTimeout  time.Duration
		checkDuration time.Duration
		expected      bool
	}{
		{
			name: "basic case",
			checkers: []checkFunc{
				func(ctx context.Context) CheckResult {
					return CheckResult{}
				},
			},
			checkTimeout:  100 * time.Millisecond,
			checkDuration: 200 * time.Millisecond,
			expected:      true,
		},
		{
			name: "long check",
			checkers: []checkFunc{
				func(ctx context.Context) CheckResult {
					<-time.After(300 * time.Millisecond)
					return CheckResult{}
				},
			},
			checkTimeout:  100 * time.Millisecond,
			checkDuration: 100 * time.Millisecond,
			expected:      false,
		},
		{
			name: "failed check",
			checkers: []checkFunc{
				func(ctx context.Context) CheckResult {
					return CheckResult{Err: errors.New("failed check")}
				},
			},
			checkTimeout:  100 * time.Millisecond,
			checkDuration: 100 * time.Millisecond,
			expected:      false,
		},
		{
			name:          "no checks",
			checkers:      nil,
			checkTimeout:  100 * time.Millisecond,
			checkDuration: 100 * time.Millisecond,
			expected:      true,
		},
	}
	for _, testcase := range testcases {
		tc := testcase
		t.Run(tc.name, func(t *testing.T) {
			g := NewCheckGroup(tc.checkTimeout)
			for i, check := range tc.checkers {
				g.AddChecker(fmt.Sprintf("subsystem%d", i), check)
			}
			ctx, cancel := context.WithTimeout(context.Background(), tc.checkDuration)
			defer cancel()
			g.Check(ctx)
			assert.Equal(t, tc.expected, g.IsOK())
		})
	}
}
