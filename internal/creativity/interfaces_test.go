package creativity

import (
	"testing"

	"github.com/google/go-github/v68/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertGitHubIssues(t *testing.T) {
	t.Run("converts issues with labels", func(t *testing.T) {
		labelName := "agent:ready"
		ghIssues := []*github.Issue{
			{
				Number: github.Ptr(1),
				Title:  github.Ptr("Issue one"),
				Body:   github.Ptr("Body one"),
				State:  github.Ptr("open"),
				Labels: []*github.Label{
					{Name: &labelName},
				},
			},
			{
				Number: github.Ptr(2),
				Title:  github.Ptr("Issue two"),
				Body:   github.Ptr("Body two"),
				State:  github.Ptr("closed"),
			},
		}

		result := convertGitHubIssues(ghIssues)

		require.Len(t, result, 2)
		assert.Equal(t, 1, result[0].Number)
		assert.Equal(t, "Issue one", result[0].Title)
		assert.Equal(t, "Body one", result[0].Body)
		assert.Equal(t, "open", result[0].State)
		assert.Equal(t, []string{"agent:ready"}, result[0].Labels)

		assert.Equal(t, 2, result[1].Number)
		assert.Equal(t, "Issue two", result[1].Title)
		assert.Equal(t, "closed", result[1].State)
		assert.Empty(t, result[1].Labels)
	})

	t.Run("empty input", func(t *testing.T) {
		result := convertGitHubIssues(nil)
		assert.Empty(t, result)
	})

	t.Run("nil fields handled gracefully", func(t *testing.T) {
		ghIssues := []*github.Issue{
			{}, // All nil fields
		}
		result := convertGitHubIssues(ghIssues)
		require.Len(t, result, 1)
		assert.Equal(t, 0, result[0].Number)
		assert.Equal(t, "", result[0].Title)
	})
}

func TestNormalizeResponse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "strips bold markers from title",
			input: "**TITLE:** My Title\n**BODY:**\nSome body",
			want:  "TITLE: My Title\nBODY:\nSome body",
		},
		{
			name:  "strips hash headers",
			input: "## TITLE: My Title\n## BODY:\nSome body",
			want:  "TITLE: My Title\nBODY:\nSome body",
		},
		{
			name:  "leaves non-marker lines unchanged",
			input: "Some text\nTITLE: Hello\nMore text\nBODY:\nContent",
			want:  "Some text\nTITLE: Hello\nMore text\nBODY:\nContent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeResponse(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTrimSpace(t *testing.T) {
	assert.Equal(t, "hello", trimSpace("  hello  "))
	assert.Equal(t, "hello", trimSpace("\n\thello\n\t"))
	assert.Equal(t, "hello world", trimSpace("  hello world  "))
	assert.Equal(t, "", trimSpace(""))
	assert.Equal(t, "", trimSpace("   "))
	assert.Equal(t, "", trimSpace("\n\t\r"))
}

func TestParseSuggestion(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantTitle string
		wantBody  string
		wantErr   string
	}{
		{
			name:      "exact format",
			input:     "TITLE: Add caching layer\nBODY:\nAdd a Redis caching layer to reduce API calls.",
			wantTitle: "Add caching layer",
			wantBody:  "Add a Redis caching layer to reduce API calls.",
		},
		{
			name:      "lowercase markers",
			input:     "title: add caching layer\nbody:\nadd a Redis caching layer.",
			wantTitle: "add caching layer",
			wantBody:  "add a Redis caching layer.",
		},
		{
			name:      "mixed case markers",
			input:     "Title: Add Caching Layer\nBody:\nAdd a Redis caching layer.",
			wantTitle: "Add Caching Layer",
			wantBody:  "Add a Redis caching layer.",
		},
		{
			name:      "markdown bold markers",
			input:     "**TITLE:** Add caching layer\n**BODY:**\nAdd a Redis caching layer.",
			wantTitle: "Add caching layer",
			wantBody:  "Add a Redis caching layer.",
		},
		{
			name:      "hash header markers",
			input:     "## TITLE: Add caching layer\n## BODY:\nAdd a Redis caching layer.",
			wantTitle: "Add caching layer",
			wantBody:  "Add a Redis caching layer.",
		},
		{
			name:      "single hash header markers",
			input:     "# TITLE: Add caching layer\n# BODY:\nAdd a Redis caching layer.",
			wantTitle: "Add caching layer",
			wantBody:  "Add a Redis caching layer.",
		},
		{
			name:      "extra whitespace before markers",
			input:     "\n\n  TITLE: Add caching layer\n\n  BODY:\nAdd a Redis caching layer.",
			wantTitle: "Add caching layer",
			wantBody:  "Add a Redis caching layer.",
		},
		{
			name:      "multiline body",
			input:     "TITLE: Add caching layer\nBODY:\nLine one.\n\nLine two.\n\n- Bullet point",
			wantTitle: "Add caching layer",
			wantBody:  "Line one.\n\nLine two.\n\n- Bullet point",
		},
		{
			name:      "no space after title colon",
			input:     "TITLE:Add caching layer\nBODY:\nSome body text.",
			wantTitle: "Add caching layer",
			wantBody:  "Some body text.",
		},
		{
			name:    "missing title",
			input:   "BODY:\nSome body text.",
			wantErr: "missing TITLE or BODY section",
		},
		{
			name:    "missing body",
			input:   "TITLE: Something",
			wantErr: "missing TITLE or BODY section",
		},
		{
			name:    "empty title",
			input:   "TITLE: \nBODY:\nSome body text.",
			wantErr: "empty title or body",
		},
		{
			name:    "empty body",
			input:   "TITLE: Something\nBODY:\n",
			wantErr: "empty title or body",
		},
		{
			name:    "completely empty input",
			input:   "",
			wantErr: "missing TITLE or BODY section",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSuggestion(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantTitle, got.Title)
			assert.Equal(t, tt.wantBody, got.Body)
		})
	}
}
