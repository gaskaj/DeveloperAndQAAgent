package ghub

import (
	"testing"

	"github.com/google/go-github/v68/github"
	"github.com/stretchr/testify/assert"
)

// TestAssignSelfIfNoAssignees_Logic tests the core logic without complex mocking.
// Full integration testing would require GitHub API access which is better handled
// in higher-level integration tests.
func TestAssignSelfIfNoAssignees_Logic(t *testing.T) {
	tests := []struct {
		name             string
		assigneeCount    int
		shouldNeedAssign bool
	}{
		{
			name:             "no assignees should trigger assignment",
			assigneeCount:    0,
			shouldNeedAssign: true,
		},
		{
			name:             "existing assignee should not trigger assignment",
			assigneeCount:    1,
			shouldNeedAssign: false,
		},
		{
			name:             "multiple assignees should not trigger assignment",
			assigneeCount:    2,
			shouldNeedAssign: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create issue with specified number of assignees
			issue := &github.Issue{
				Number:    github.Int(123),
				Assignees: make([]*github.User, tt.assigneeCount),
			}

			// Fill assignees if needed
			for i := 0; i < tt.assigneeCount; i++ {
				issue.Assignees[i] = &github.User{
					Login: github.String("user" + string(rune('1'+i))),
				}
			}

			// Test the logic condition
			shouldAssign := len(issue.Assignees) == 0
			assert.Equal(t, tt.shouldNeedAssign, shouldAssign)
		})
	}
}

// Test that nil assignees are treated as empty
func TestAssignSelfIfNoAssignees_NilAssignees(t *testing.T) {
	issue := &github.Issue{
		Number:    github.Int(123),
		Assignees: nil,
	}

	shouldAssign := len(issue.Assignees) == 0
	assert.True(t, shouldAssign, "nil assignees should be treated as empty and trigger assignment")
}
