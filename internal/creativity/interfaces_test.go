package creativity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
