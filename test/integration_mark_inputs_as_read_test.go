package test_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/test/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFixtureMarkInputsAsRead = "fixtures/mark-inputs-as-read"

// TestMarkInputsAsRead covers the mark-inputs-as-read experiment end-to-end: a reading= filter
// must match a template marked from inputs only when the experiment is on, and the best-effort
// decode of unresolved inputs must not drop the unit from discovery.
func TestMarkInputsAsRead(t *testing.T) {
	t.Parallel()

	workingDir, err := filepath.Abs(testFixtureMarkInputsAsRead)
	require.NoError(t, err)

	testCases := []struct {
		name          string
		args          string
		expectedUnits []string
	}{
		{
			name:          "experiment enabled records the read declared in inputs",
			args:          "--filter 'reading=unit/policy.json.tftpl' --experiment mark-inputs-as-read",
			expectedUnits: []string{"unit"},
		},
		{
			name:          "experiment disabled leaves inputs unevaluated",
			args:          "--filter 'reading=unit/policy.json.tftpl'",
			expectedUnits: []string{},
		},
		{
			name:          "unresolved inputs do not drop the unit from discovery",
			args:          "--reading --experiment mark-inputs-as-read",
			expectedUnits: []string{"unit", "vpc"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			helpers.CleanupTerraformFolder(t, workingDir)

			cmd := "terragrunt find --no-color --working-dir " + workingDir + " " + tc.args
			stdout, stderr, err := helpers.RunTerragruntCommandWithOutput(t, cmd)
			require.NoError(t, err, "stderr: %s", stderr)

			assert.ElementsMatch(t, tc.expectedUnits, strings.Fields(stdout))
		})
	}
}
