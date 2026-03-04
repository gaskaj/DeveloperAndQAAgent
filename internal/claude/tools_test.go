package claude

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDevTools_ReturnsAllTools(t *testing.T) {
	tools := DevTools()
	require.Len(t, tools, 6, "DevTools should return exactly 6 tools")
}

func TestDevTools_ToolNames(t *testing.T) {
	tools := DevTools()

	expectedNames := []string{
		"read_file",
		"edit_file",
		"write_file",
		"search_files",
		"list_files",
		"run_command",
	}

	for i, expected := range expectedNames {
		require.NotNil(t, tools[i].OfTool, "tool %d should have OfTool set", i)
		assert.Equal(t, expected, tools[i].OfTool.Name, "tool %d should be %s", i, expected)
	}
}

func TestDevTools_AllHaveDescriptions(t *testing.T) {
	tools := DevTools()
	for _, tool := range tools {
		require.NotNil(t, tool.OfTool)
		desc := tool.OfTool.Description
		assert.NotZero(t, desc, "tool %s should have a description", tool.OfTool.Name)
	}
}

func TestDevTools_AllHaveRequiredFields(t *testing.T) {
	tools := DevTools()
	for _, tool := range tools {
		require.NotNil(t, tool.OfTool)
		assert.NotEmpty(t, tool.OfTool.InputSchema.Required,
			"tool %s should have at least one required field", tool.OfTool.Name)
	}
}

func TestDevTools_AllHaveProperties(t *testing.T) {
	tools := DevTools()
	for _, tool := range tools {
		require.NotNil(t, tool.OfTool)
		assert.NotEmpty(t, tool.OfTool.InputSchema.Properties,
			"tool %s should have properties defined", tool.OfTool.Name)
	}
}

func TestDevTools_ReadFile(t *testing.T) {
	tools := DevTools()
	tool := tools[0].OfTool
	assert.Equal(t, "read_file", tool.Name)
	assert.Contains(t, tool.InputSchema.Required, "path")

	props, ok := tool.InputSchema.Properties.(map[string]interface{})
	require.True(t, ok, "Properties should be map[string]interface{}")
	_, hasPath := props["path"]
	assert.True(t, hasPath, "read_file should have path property")
}

func TestDevTools_EditFile(t *testing.T) {
	tools := DevTools()
	tool := tools[1].OfTool
	assert.Equal(t, "edit_file", tool.Name)
	assert.Contains(t, tool.InputSchema.Required, "path")
	assert.Contains(t, tool.InputSchema.Required, "old_string")
	assert.Contains(t, tool.InputSchema.Required, "new_string")
	assert.Len(t, tool.InputSchema.Required, 3)
}

func TestDevTools_WriteFile(t *testing.T) {
	tools := DevTools()
	tool := tools[2].OfTool
	assert.Equal(t, "write_file", tool.Name)
	assert.Contains(t, tool.InputSchema.Required, "path")
	assert.Contains(t, tool.InputSchema.Required, "content")
}

func TestDevTools_SearchFiles(t *testing.T) {
	tools := DevTools()
	tool := tools[3].OfTool
	assert.Equal(t, "search_files", tool.Name)
	assert.Contains(t, tool.InputSchema.Required, "pattern")

	props, ok := tool.InputSchema.Properties.(map[string]interface{})
	require.True(t, ok, "Properties should be map[string]interface{}")
	_, hasPath := props["path"]
	assert.True(t, hasPath, "search_files should have optional path property")
}

func TestDevTools_ListFiles(t *testing.T) {
	tools := DevTools()
	tool := tools[4].OfTool
	assert.Equal(t, "list_files", tool.Name)
	assert.Contains(t, tool.InputSchema.Required, "path")
}

func TestDevTools_RunCommand(t *testing.T) {
	tools := DevTools()
	tool := tools[5].OfTool
	assert.Equal(t, "run_command", tool.Name)
	assert.Contains(t, tool.InputSchema.Required, "command")
}

func TestDevTools_Idempotent(t *testing.T) {
	tools1 := DevTools()
	tools2 := DevTools()
	require.Len(t, tools1, len(tools2))
	for i := range tools1 {
		assert.Equal(t, tools1[i].OfTool.Name, tools2[i].OfTool.Name)
	}
}
