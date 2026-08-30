package test_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/cli/commands/find"
	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testFixtureMarkReadFunctions = "fixtures/mark-read-functions/"
	markReadFunctionsExperiment  = "--experiment mark-read-functions"
)

func copyMarkReadFunctionsFixture(t *testing.T) string {
	t.Helper()

	tmpEnvPath := helpers.CopyEnvironment(t, testFixtureMarkReadFunctions)
	rootPath := filepath.Join(tmpEnvPath, testFixtureMarkReadFunctions)
	rootPath, err := filepath.EvalSymlinks(rootPath)
	require.NoError(t, err)

	return rootPath
}

// TestMarkReadFunctionsRootUnitReadingFilter verifies that reads from reusable
// units included into main resolve to the units' own absolute paths — a reading=
// filter hits the unit's real path (and main via the include), never a path
// anchored to main's directory.
func TestMarkReadFunctionsRootUnitReadingFilter(t *testing.T) {
	t.Parallel()

	rootPath := copyMarkReadFunctionsFixture(t)

	reusableA := filepath.Join(rootPath, "reusable-a", "settings.yaml")
	reusableB := filepath.Join(rootPath, "reusable-b", "config.json")
	unused := filepath.Join(rootPath, "unused", "unused.yaml")

	testCases := []struct {
		name          string
		filterQuery   string
		expectedUnits []string
	}{
		{
			name:          "absolute path to reusable-a data file selects reusable-a and main",
			filterQuery:   "reading=" + reusableA,
			expectedUnits: []string{"main", "reusable-a"},
		},
		{
			name:          "absolute path to reusable-b data file selects reusable-b and main",
			filterQuery:   "reading=" + reusableB,
			expectedUnits: []string{"main", "reusable-b"},
		},
		{
			name:          "relative path resolves to reusable-a data file selects reusable-a and main",
			filterQuery:   "reading=reusable-a/settings.yaml",
			expectedUnits: []string{"main", "reusable-a"},
		},
		{
			name:          "main-anchored reusable-a path selects nothing",
			filterQuery:   "reading=" + filepath.Join(rootPath, "main", "settings.yaml"),
			expectedUnits: []string{},
		},
		{
			name:          "main-anchored reusable-b path selects nothing",
			filterQuery:   "reading=" + filepath.Join(rootPath, "main", "config.json"),
			expectedUnits: []string{},
		},
		{
			name:          "file read by no unit selects nothing",
			filterQuery:   "reading=" + unused,
			expectedUnits: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cmd := "terragrunt find --no-color --working-dir " + rootPath + " " +
				markReadFunctionsExperiment + " --filter '" + tc.filterQuery + "'"
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err, "stderr: %s", stderr)

			assert.ElementsMatch(t, tc.expectedUnits, strings.Fields(stdout),
				"output mismatch for filter query: %s", tc.filterQuery)
		})
	}
}

// TestMarkReadFunctionsReadingJSON asserts the exact reading paths in the
// find --json --reading output: each unit's file() call resolves to its own
// folder's absolute location, never one anchored to main's directory.
func TestMarkReadFunctionsReadingJSON(t *testing.T) {
	t.Parallel()

	rootPath := copyMarkReadFunctionsFixture(t)

	cmd := "terragrunt find --no-color --json --reading --working-dir " + rootPath + " " +
		markReadFunctionsExperiment
	stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
	require.NoError(t, err, "stderr: %s", stderr)

	var components find.FoundComponents
	require.NoError(t, json.Unmarshal([]byte(stdout), &components))

	readingsFor := func(path string) []string {
		for _, c := range components {
			if c.Path == path {
				return c.Reading
			}
		}
		return nil
	}

	t.Run("reusable-a is recorded against its own folder", func(t *testing.T) {
		t.Parallel()
		read := readingsFor("reusable-a")
		assert.Contains(t, read, "reusable-a/settings.yaml")
		assert.NotContains(t, read, "main/settings.yaml")
	})

	t.Run("reusable-b is recorded against its own folder", func(t *testing.T) {
		t.Parallel()
		read := readingsFor("reusable-b")
		assert.Contains(t, read, "reusable-b/config.json")
		assert.NotContains(t, read, "main/config.json")
	})

	t.Run("main records the reusable units' correct paths, not main-anchored ones", func(t *testing.T) {
		t.Parallel()
		read := readingsFor("main")
		assert.Contains(t, read, "reusable-a/settings.yaml")
		assert.Contains(t, read, "reusable-b/config.json")
		assert.NotContains(t, read, "main/settings.yaml")
		assert.NotContains(t, read, "main/config.json")
	})

	t.Run("unused reads nothing", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, readingsFor("unused"))
	})
}
