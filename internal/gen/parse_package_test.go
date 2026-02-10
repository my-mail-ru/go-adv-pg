//go:build !integration

package advpggen_test

import (
	"cmp"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	gocmp "github.com/google/go-cmp/cmp"

	advpggen "github.com/my-mail-ru/go-adv-pg/internal/gen"
)

type testFS []string

var _ fs.GlobFS = testFS{}

func (testFS) Open(string) (fs.File, error) {
	panic("testFS.Open should not be called")
}

func (tfs testFS) Glob(string) ([]string, error) {
	return tfs, nil
}

func cmpSlices[T cmp.Ordered](x, y []T) bool {
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

func TestExcludeFS(t *testing.T) {
	tests := []struct {
		name string
		in   testFS
		want []string
	}{{
		name: "no excluded file",
		in:   testFS{"abc", "def"},
		want: []string{"abc", "def"},
	}, {
		name: "excluded first",
		in:   testFS{"excluded", "abc", "def"},
		want: []string{"abc", "def"},
	}, {
		name: "excluded in the middle",
		in:   testFS{"abc", "excluded", "def"},
		want: []string{"abc", "def"},
	}, {
		name: "multiple excluded in a row", // not a real case
		in:   testFS{"abc", "excluded", "excluded", "def"},
		want: []string{"abc", "def"},
	}, {
		name: "excluded last",
		in:   testFS{"abc", "def", "excluded"},
		want: []string{"abc", "def"},
	}, {
		name: "multiple excluded at the end", // not a real case
		in:   testFS{"abc", "def", "excluded", "excluded"},
		want: []string{"abc", "def"},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			efs := advpggen.NewExcludeFS(tt.in, "excluded")
			got, err := fs.Glob(efs, "*")
			if err != nil {
				t.Fatal("fs.Glob: ", err)
			}

			if !cmpSlices(got, tt.want) {
				t.Errorf("got: %v, want: %v", got, tt.want)
			}
		})
	}
}

func TestParsePackage(t *testing.T) {
	tests := []struct {
		name    string
		files   map[string]string
		want    advpggen.Package
		wantErr string
	}{{
		name: "broken file",
		files: map[string]string{
			"broken.go": "const foo=bar",
		},
		wantErr: "expected 'package'",
	}, {
		name: "const without a value",
		files: map[string]string{
			"novalue.go": "package test\nconst PackageDAO",
		},
		wantErr: "const PackageDAO should have a value",
	}, {
		name: "unsupported const expr",
		files: map[string]string{
			"const.go": "package test\nconst PackageDAO = Prefix + OtherConst",
		},
		wantErr: "const PackageDAO must be a simple string literal",
	}, {
		name: "non-string const",
		files: map[string]string{
			"novalue.go": "package test\nconst PackageDAO = 123",
		},
		wantErr: "const PackageDAO must be a simple string literal",
	}, {
		name: "without PackageDAO const",
		files: map[string]string{
			"test.dat":     "not a go file",
			"struct.go":    "package test\ntype Test struct {}\ntype Alias = int\nconst zero = 0",
			"nonstruct.go": "package test\ntype Int int\nfunc foo(){}",
		},
		want: advpggen.Package{
			AllTypes: advpggen.AllTypes{"Test": true, "Int": false},
		},
	}, {
		name: "with PackageDAO const",
		files: map[string]string{
			"struct.go":    "package test\ntype Test struct {}\ntype Alias = int\nconst PackageDAO = `Test`",
			"nonstruct.go": "package test\ntype Int int",
		},
		want: advpggen.Package{
			AllTypes:   advpggen.AllTypes{"Test": true, "Int": false},
			PackageDAO: "Test",
		},
	}, {
		name: "weird const definition",
		files: map[string]string{
			"multi_const.go": "package test\nconst (\nFooBar = 123\nTest, PackageDAO, _ = `test`, `Test`, `baz`\n)",
		},
		want: advpggen.Package{
			AllTypes:   advpggen.AllTypes{},
			PackageDAO: "Test",
		},
	}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsys := make(fstest.MapFS, len(tt.files))

			for fname, data := range tt.files {
				fsys[fname] = &fstest.MapFile{Data: []byte(data)}
			}

			got, err := advpggen.ParsePackage(advpggen.NewFileSet(), fsys)
			if err != nil {
				if tt.wantErr == "" {
					t.Fatal("unexpected error: ", err)
				}

				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("got error %v, want error %q", err, tt.wantErr)
				}

				return
			}

			if diff := gocmp.Diff(tt.want, got); diff != "" {
				t.Error("ParseDAO: result mismatch (-want +got):\n" + diff)
			}
		})
	}
}
