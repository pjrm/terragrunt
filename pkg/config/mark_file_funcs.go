package config

import (
	"maps"
	"path/filepath"

	"github.com/hashicorp/terraform/lang/funcs"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// markReadFileFunctions wraps Terraform's file functions so the paths they
// touch are recorded as read like an implicit mark_as_read, letting
// reading-based filters and discovery detect them without an explicit call.
func markReadFileFunctions(
	pctx *ParsingContext,
	baseDir string,
	functions map[string]function.Function,
) {
	maps.Copy(functions, fileFuncWrappers(pctx, baseDir, functions))
}

// fileFuncWrappers returns a marked wrapper for every Terraform file function.
func fileFuncWrappers(
	pctx *ParsingContext,
	baseDir string,
	functions map[string]function.Function,
) map[string]function.Function {
	mark := func(pathVal cty.Value) {
		recordRead(pctx, baseDir, pathVal.AsString())
	}

	return map[string]function.Function{
		"file":             wrapSinglePathFileFunc(baseDir, mark, false),
		"filebase64":       wrapSinglePathFileFunc(baseDir, mark, true),
		"fileexists":       wrapFileExistsFunc(baseDir, mark),
		"filemd5":          wrapFileHashFunc(baseDir, mark, "md5"),
		"filesha1":         wrapFileHashFunc(baseDir, mark, "sha1"),
		"filesha256":       wrapFileHashFunc(baseDir, mark, "sha256"),
		"filesha512":       wrapFileHashFunc(baseDir, mark, "sha512"),
		"filebase64sha256": wrapFileHashFunc(baseDir, mark, "base64sha256"),
		"filebase64sha512": wrapFileHashFunc(baseDir, mark, "base64sha512"),
		"templatefile": wrapTemplateFileFunc(pctx, baseDir, func() map[string]function.Function {
			return functions
		}),
		"fileset": wrapFileSetFunc(pctx, baseDir),
	}
}

// recordRead records path as read. Relative paths anchor to baseDir — where the
// file function resolves them — not the run's working directory, so an included
// config's reads point at its own folder rather than the including unit's.
func recordRead(pctx *ParsingContext, baseDir, path string) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
		path = filepath.Clean(path)
	}

	pctx.FilesRead.Add(path)
}

func wrapSinglePathFileFunc(
	baseDir string,
	mark func(cty.Value),
	encBase64 bool,
) function.Function {
	orig := funcs.MakeFileFunc(baseDir, encBase64)

	return function.New(&function.Spec{
		Params: orig.Params(),
		Type:   orig.ReturnTypeForValues,
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			if !args[0].IsNull() && args[0].IsKnown() {
				mark(args[0])
			}

			return orig.Call(args)
		},
	})
}

func wrapFileExistsFunc(
	baseDir string,
	mark func(cty.Value),
) function.Function {
	orig := funcs.MakeFileExistsFunc(baseDir)

	return function.New(&function.Spec{
		Params: orig.Params(),
		Type:   orig.ReturnTypeForValues,
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			if !args[0].IsNull() && args[0].IsKnown() {
				mark(args[0])
			}

			return orig.Call(args)
		},
	})
}

func wrapFileHashFunc(
	baseDir string,
	mark func(cty.Value),
	kind string,
) function.Function {
	var orig function.Function

	switch kind {
	case "md5":
		orig = funcs.MakeFileMd5Func(baseDir)
	case "sha1":
		orig = funcs.MakeFileSha1Func(baseDir)
	case "sha256":
		orig = funcs.MakeFileSha256Func(baseDir)
	case "sha512":
		orig = funcs.MakeFileSha512Func(baseDir)
	case "base64sha256":
		orig = funcs.MakeFileBase64Sha256Func(baseDir)
	case "base64sha512":
		orig = funcs.MakeFileBase64Sha512Func(baseDir)
	}

	return function.New(&function.Spec{
		Params: orig.Params(),
		Type:   orig.ReturnTypeForValues,
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			if !args[0].IsNull() && args[0].IsKnown() {
				mark(args[0])
			}

			return orig.Call(args)
		},
	})
}

// funcsCb hands templates the full marked map so they can call the other marked
// functions; MakeTemplateFileFunc stubs out recursive templatefile calls.
func wrapTemplateFileFunc(
	pctx *ParsingContext,
	baseDir string,
	funcsCb func() map[string]function.Function,
) function.Function {
	orig := funcs.MakeTemplateFileFunc(baseDir, funcsCb)

	mark := func(pathVal cty.Value) {
		if !pathVal.IsNull() && pathVal.IsKnown() {
			recordRead(pctx, baseDir, pathVal.AsString())
		}
	}

	return function.New(&function.Spec{
		Params: orig.Params(),
		Type: func(args []cty.Value) (cty.Type, error) {
			mark(args[0])
			return orig.ReturnTypeForValues(args)
		},
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			mark(args[0])
			return orig.Call(args)
		},
	})
}

// wrapFileSetFunc records the matches fileset returns as read.
func wrapFileSetFunc(
	pctx *ParsingContext,
	baseDir string,
) function.Function {
	orig := funcs.MakeFileSetFunc(baseDir)

	return function.New(&function.Spec{
		Params: orig.Params(),
		Type:   orig.ReturnTypeForValues,
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			out, err := orig.Call(args)
			if err != nil {
				return out, err
			}

			// fileset returns the matched paths relative to its path argument,
			// so re-anchor each back to the resolved directory.
			dir := args[0].AsString()
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(baseDir, dir)
			}

			for it := out.ElementIterator(); it.Next(); {
				_, rel := it.Element()
				pctx.FilesRead.Add(filepath.Join(dir, filepath.FromSlash(rel.AsString())))
			}

			return out, nil
		},
	})
}
