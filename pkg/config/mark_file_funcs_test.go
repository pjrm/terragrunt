package config_test

import (
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/internal/experiment"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/test/helpers/logger"
	"github.com/gruntwork-io/terragrunt/test/helpers/venvtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMarkReadFunctions_FileFuncs verifies that the single-path Terraform file
// functions mark their target as read when the experiment is enabled.
func TestMarkReadFunctions_FileFuncs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "input.txt"), "hello")

	l := logger.CreateLogger()
	configPath := filepath.Join(dir, config.DefaultTerragruntConfigPath)
	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), configPath)
	pctx.WorkingDir = dir
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.MarkReadFunctions))

	hcl := `locals {
  a = file("input.txt")
  b = fileexists("input.txt")
  c = filebase64("input.txt")
  d = filesha256("input.txt")
}`

	out, err := config.ParseConfigString(ctx, pctx, l, configPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out)

	require.Equal(t, "hello", out.Locals["a"], "file() content")
	require.Equal(t, "aGVsbG8=", out.Locals["c"], "filebase64() content")
	require.Equal(t, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", out.Locals["d"], "filesha256() content")

	require.NotNil(t, pctx.FilesRead)
	read := pctx.FilesRead.Paths()
	assert.Contains(t, read, filepath.Join(dir, "input.txt"))
}

// TestMarkReadFunctions_Disabled verifies that without the experiment the file
// functions do NOT mark files as read, preserving the historical behavior.
func TestMarkReadFunctions_Disabled(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "input.txt"), "hello")

	l := logger.CreateLogger()
	configPath := filepath.Join(dir, config.DefaultTerragruntConfigPath)
	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), configPath)
	pctx.WorkingDir = dir

	hcl := `locals { a = file("input.txt") }`

	out, err := config.ParseConfigString(ctx, pctx, l, configPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out)

	assert.Empty(t, pctx.FilesRead.Paths())
}

// TestMarkReadFunctions_TemplateFile verifies that templatefile marks the
// referenced template as read.
func TestMarkReadFunctions_TemplateFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "template.tmpl"), "hello ${x}")

	l := logger.CreateLogger()
	configPath := filepath.Join(dir, config.DefaultTerragruntConfigPath)
	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), configPath)
	pctx.WorkingDir = dir
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.MarkReadFunctions))

	hcl := `locals { a = templatefile("template.tmpl", { x = "world" }) }`

	out, err := config.ParseConfigString(ctx, pctx, l, configPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out)

	require.Equal(t, "hello world", out.Locals["a"], "templatefile() content")

	read := pctx.FilesRead.Paths()
	assert.Contains(t, read, filepath.Join(dir, "template.tmpl"))
}

// TestMarkReadFunctions_FileSet verifies that fileset marks each matched file
// as read.
func TestMarkReadFunctions_FileSet(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.yaml"), "")
	writeFile(t, filepath.Join(dir, "b.yaml"), "")
	writeFile(t, filepath.Join(dir, "README.md"), "")

	l := logger.CreateLogger()
	configPath := filepath.Join(dir, config.DefaultTerragruntConfigPath)
	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), configPath)
	pctx.WorkingDir = dir
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.MarkReadFunctions))

	hcl := `locals { matches = fileset(".", "*.yaml") }`

	out, err := config.ParseConfigString(ctx, pctx, l, configPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out)

	require.ElementsMatch(t, []string{"a.yaml", "b.yaml"}, out.Locals["matches"],
		"fileset() result")

	read := pctx.FilesRead.Paths()
	assert.Contains(t, read, filepath.Join(dir, "a.yaml"))
	assert.Contains(t, read, filepath.Join(dir, "b.yaml"))
	assert.NotContains(t, read, filepath.Join(dir, "README.md"))
}

// TestMarkReadFunctions_AbsolutePath verifies that an absolute path passed to
// a file function is recorded as-is.
func TestMarkReadFunctions_AbsolutePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	abs := filepath.Join(dir, "input.txt")
	writeFile(t, abs, "hello")

	l := logger.CreateLogger()
	configPath := filepath.Join(dir, config.DefaultTerragruntConfigPath)
	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), configPath)
	pctx.WorkingDir = dir
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.MarkReadFunctions))

	hcl := `locals { a = file("` + abs + `") }`

	out, err := config.ParseConfigString(ctx, pctx, l, configPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out)

	read := pctx.FilesRead.Paths()
	assert.Contains(t, read, abs)
}

// TestMarkReadFunctions_LocalVariable verifies that a path passed to a file
// function through a local variable resolves to a concrete string before the
// wrapper records it, so the resolved path is tracked rather than the raw
// reference.
func TestMarkReadFunctions_LocalVariable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "shared.json"), `{"a": 1}`)

	l := logger.CreateLogger()
	configPath := filepath.Join(dir, config.DefaultTerragruntConfigPath)
	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), configPath)
	pctx.WorkingDir = dir
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.MarkReadFunctions))

	hcl := `locals {
  the_file = "shared.json"
  content  = file(local.the_file)
}`

	out, err := config.ParseConfigString(ctx, pctx, l, configPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out)

	require.Equal(t, `{"a": 1}`, out.Locals["content"], "file() content via local variable")

	read := pctx.FilesRead.Paths()
	assert.Contains(t, read, filepath.Join(dir, "shared.json"))
}

// TestMarkReadFunctions_LocalVariableMulti verifies that templatefile and
// fileset also resolve local variables before the wrapper records the paths.
func TestMarkReadFunctions_LocalVariableMulti(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.yaml"), "")
	writeFile(t, filepath.Join(dir, "b.yaml"), "")
	writeFile(t, filepath.Join(dir, "template.tmpl"), "hello ${x}")

	l := logger.CreateLogger()
	configPath := filepath.Join(dir, config.DefaultTerragruntConfigPath)
	ctx, pctx := newTestParsingContext(t, venvtest.NewOSWithEmptyEnv(), configPath)
	pctx.WorkingDir = dir
	require.NoError(t, pctx.Experiments.EnableExperiment(experiment.MarkReadFunctions))

	hcl := `locals {
  tmpl_name = "template.tmpl"
  glob_dir  = "."
  glob_pat  = "*.yaml"
  t         = templatefile(local.tmpl_name, { x = "world" })
  m         = fileset(local.glob_dir, local.glob_pat)
}`

	out, err := config.ParseConfigString(ctx, pctx, l, configPath, hcl, nil)
	require.NoError(t, err)
	require.NotNil(t, out)

	read := pctx.FilesRead.Paths()
	assert.Contains(t, read, filepath.Join(dir, "template.tmpl"))
	assert.Contains(t, read, filepath.Join(dir, "a.yaml"))
	assert.Contains(t, read, filepath.Join(dir, "b.yaml"))
}
