package stacks_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/apppackio/apppack/stacks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// templateURLStacks lists every Stack implementation with the template file
// name its TemplateURL resolves to. Add new stack types here;
// TestTemplateURLTableIsComplete fails if one is missing.
var templateURLStacks = map[string]struct {
	stack    stacks.Stack
	template string
}{
	"AppStack":          {&stacks.AppStack{}, "app.json"},
	"ClusterStack":      {&stacks.ClusterStack{}, "cluster.json"},
	"DatabaseStack":     {&stacks.DatabaseStack{}, "database.json"},
	"RedisStack":        {&stacks.RedisStack{}, "redis.json"},
	"AccountStack":      {&stacks.AccountStack{}, "account.json"},
	"RegionStack":       {&stacks.RegionStack{}, "region.json"},
	"CustomDomainStack": {&stacks.CustomDomainStack{}, "custom-domain.json"},
	"ReviewAppStack":    {&stacks.ReviewAppStack{}, "review-app.json"},
}

const templateBase = "https://s3.amazonaws.com/apppack-cloudformations/"

func TestTemplateURLConsistentAcrossStacks(t *testing.T) {
	t.Parallel()

	canary := "canary"
	empty := ""
	releases := []struct {
		name    string
		release *string
		dir     string
	}{
		{"nil release", nil, "latest"},
		{"empty release", &empty, "latest"},
		{"named release", &canary, "canary"},
	}

	for stackName, tc := range templateURLStacks {
		for _, r := range releases {
			t.Run(stackName+"/"+r.name, func(t *testing.T) {
				t.Parallel()

				got := tc.stack.TemplateURL(r.release)
				require.NotNil(t, got)
				assert.Equal(t, templateBase+r.dir+"/"+tc.template, *got)
			})
		}
	}
}

// TestTemplateURLTableIsComplete fails when a type with a TemplateURL method
// is declared in this package but missing from templateURLStacks, so a new
// stack type cannot skip the consistency test above.
func TestTemplateURLTableIsComplete(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob("*.go")
	require.NoError(t, err)

	fset := token.NewFileSet()
	files := make([]*ast.File, 0, len(paths))

	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, path, nil, 0)
		require.NoError(t, err)

		files = append(files, file)
	}

	require.NotEmpty(t, files, "no stacks source files found")

	var found []string

	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "TemplateURL" {
				continue
			}

			typ := fn.Recv.List[0].Type
			if star, ok := typ.(*ast.StarExpr); ok {
				typ = star.X
			}

			if ident, ok := typ.(*ast.Ident); ok {
				found = append(found, ident.Name)
			}
		}
	}

	var want []string
	for name := range templateURLStacks {
		want = append(want, name)
	}

	sort.Strings(found)
	sort.Strings(want)
	assert.Equal(t, want, found, "templateURLStacks must list every type implementing TemplateURL")
}
