package memory

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".memory")
	store, err := NewStore(dir)
	require.NoError(t, err)
	assert.Equal(t, 0, store.Count())
}

func TestStoreAddAndRetrieve(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".memory")
	store, err := NewStore(dir)
	require.NoError(t, err)

	err = store.Add(&Entry{
		Category: CategoryArchitecture,
		Content:  "Uses dependency injection via agent.Dependencies struct",
	})
	require.NoError(t, err)
	assert.Equal(t, 1, store.Count())

	entries := store.All()
	assert.Len(t, entries, 1)
	assert.Equal(t, CategoryArchitecture, entries[0].Category)
}

func TestStoreDeduplicates(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".memory")
	store, err := NewStore(dir)
	require.NoError(t, err)

	content := "Uses chi router for HTTP handling"
	_ = store.Add(&Entry{Category: CategoryPattern, Content: content})
	_ = store.Add(&Entry{Category: CategoryPattern, Content: content})

	assert.Equal(t, 1, store.Count())
}

func TestStoreByCategory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".memory")
	store, err := NewStore(dir)
	require.NoError(t, err)

	_ = store.Add(&Entry{Category: CategoryArchitecture, Content: "arch1"})
	_ = store.Add(&Entry{Category: CategoryConvention, Content: "conv1"})
	_ = store.Add(&Entry{Category: CategoryArchitecture, Content: "arch2"})

	arch := store.ByCategory(CategoryArchitecture)
	assert.Len(t, arch, 2)
	conv := store.ByCategory(CategoryConvention)
	assert.Len(t, conv, 1)
}

func TestStorePersistence(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".memory")

	// Create and populate
	store1, err := NewStore(dir)
	require.NoError(t, err)
	_ = store1.Add(&Entry{Category: CategoryGotcha, Content: "Must rebuild binary after changing internal/ code"})

	// Reload from disk
	store2, err := NewStore(dir)
	require.NoError(t, err)
	assert.Equal(t, 1, store2.Count())
	assert.Equal(t, "Must rebuild binary after changing internal/ code", store2.All()[0].Content)
}

func TestFormatForPrompt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".memory")
	store, err := NewStore(dir)
	require.NoError(t, err)

	_ = store.Add(&Entry{Category: CategoryArchitecture, Content: "Uses DI via Dependencies struct"})
	_ = store.Add(&Entry{Category: CategoryConvention, Content: "Error wrapping uses fmt.Errorf with %w"})

	prompt := store.FormatForPrompt()
	assert.Contains(t, prompt, "Repository Memory")
	assert.Contains(t, prompt, "Architecture")
	assert.Contains(t, prompt, "Uses DI via Dependencies struct")
	assert.Contains(t, prompt, "Conventions")
}

func TestFormatForPromptEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".memory")
	store, err := NewStore(dir)
	require.NoError(t, err)

	prompt := store.FormatForPrompt()
	assert.Empty(t, prompt)
}
