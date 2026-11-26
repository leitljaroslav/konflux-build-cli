package tknbundle

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
)

func TestResolveFiles(t *testing.T) {
	g := NewWithT(t)
	// The source directory's glob characters must be treated literally.
	sourceDir := filepath.Join(t.TempDir(), "[source] tree")
	for _, name := range []string{
		"a.yaml", "b.yml", "literal[1].yaml", "task.json", "README.md",
		"sub/c.yaml", "sub/deep/d.yml", "sub/ignored.txt", "other/e.yaml",
		".hidden.yaml", ".hidden/secret.yaml", "looks.yaml/README.md",
	} {
		path := filepath.Join(sourceDir, name)
		g.Expect(os.MkdirAll(filepath.Dir(path), 0755)).To(Succeed())
		g.Expect(os.WriteFile(path, []byte("fixture"), 0644)).To(Succeed())
	}
	g.Expect(os.Mkdir(filepath.Join(sourceDir, "empty"), 0755)).To(Succeed())

	tests := []struct {
		name    string
		context string
		want    []string
		wantErr string
	}{
		{
			name:    "default context recursively selects YAML files",
			context: ".",
			want:    []string{"a.yaml", "literal[1].yaml", "other/e.yaml", "sub/c.yaml", "b.yml", "sub/deep/d.yml"},
		},
		{name: "single directory", context: "sub", want: []string{"sub/c.yaml", "sub/deep/d.yml"}},
		{name: "single file", context: "b.yml", want: []string{"b.yml"}},
		{name: "comma separated selections", context: "b.yml,sub", want: []string{"b.yml", "sub/c.yaml", "sub/deep/d.yml"}},
		{name: "space separated selections", context: "b.yml sub", want: []string{"b.yml", "sub/c.yaml", "sub/deep/d.yml"}},
		{name: "mixed separators", context: " , b.yml,\t sub  ,", want: []string{"b.yml", "sub/c.yaml", "sub/deep/d.yml"}},
		{
			name:    "single directory exclusion starts with all YAML files",
			context: "!sub",
			want:    []string{"a.yaml", "literal[1].yaml", "other/e.yaml", "b.yml"},
		},
		{
			name:    "single file exclusion starts with all YAML files",
			context: "!a.yaml",
			want:    []string{"literal[1].yaml", "other/e.yaml", "sub/c.yaml", "b.yml", "sub/deep/d.yml"},
		},
		{name: "exclude after inclusion", context: "sub,!sub/deep", want: []string{"sub/c.yaml"}},
		{name: "multiple exclusions", context: ".,!sub,!other,!literal[1].yaml", want: []string{"a.yaml", "b.yml"}},
		{name: "later inclusion restores excluded file", context: "sub,!sub,sub/c.yaml", want: []string{"sub/c.yaml"}},
		{name: "leading exclusion does not imply inclusion for multiple selections", context: "!sub,a.yaml", want: []string{"a.yaml"}},
		{name: "overlapping selections are retained", context: "sub,sub/c.yaml,sub", want: []string{"sub/c.yaml", "sub/deep/d.yml", "sub/c.yaml", "sub/c.yaml", "sub/deep/d.yml"}},
		{name: "exclusion removes every selected occurrence", context: "sub,sub/c.yaml,sub,!sub/c.yaml", want: []string{"sub/deep/d.yml", "sub/deep/d.yml"}},
		{name: "simple glob", context: "*.yml", want: []string{"b.yml"}},
		{name: "recursive glob", context: "**/*.yaml", want: []string{"a.yaml", "literal[1].yaml", "other/e.yaml", "sub/c.yaml"}},
		{name: "glob matching a directory", context: "su*", want: []string{"sub/c.yaml", "sub/deep/d.yml"}},
		{name: "glob exclusion", context: "sub,!**/*.yml", want: []string{"sub/c.yaml"}},
		{name: "literal file with glob characters", context: "literal[1].yaml", want: []string{"literal[1].yaml"}},
		{name: "explicit JSON file is preserved", context: "task.json", want: []string{"task.json"}},
		{name: "explicit hidden file", context: ".hidden.yaml", want: []string{".hidden.yaml"}},
		{name: "explicit hidden directory", context: ".hidden", want: []string{".hidden/secret.yaml"}},
		{name: "missing exclusion is harmless", context: "a.yaml,!missing", want: []string{"a.yaml"}},
		{name: "unmatched glob alongside matching selection", context: "a.yaml,missing*.yaml", want: []string{"a.yaml"}},
		{name: "missing file", context: "missing.yaml", wantErr: "resolving context path"},
		{name: "missing file alongside matching selection", context: "a.yaml,missing.yaml", wantErr: "resolving context path"},
		{name: "malformed glob", context: "[", wantErr: "syntax error"},
		{name: "empty context", context: "", wantErr: "no files matched"},
		{name: "only separators", context: " , \t, ", wantErr: "no files matched"},
		{name: "empty directory", context: "empty", wantErr: "no files matched"},
		{name: "directory named like a YAML file", context: "looks.yaml", wantErr: "no files matched"},
		{name: "unmatched glob", context: "missing*.yaml", wantErr: "no files matched"},
		{name: "everything excluded", context: ".,!.", wantErr: "no files matched"},
		{name: "multiple exclusions do not imply inclusion", context: "!sub,!other", wantErr: "no files matched"},
		{name: "empty exclusion", context: "!", wantErr: "exclusion must specify a path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			files, err := resolveFiles(sourceDir, tt.context)
			if tt.wantErr != "" {
				g.Expect(err).To(MatchError(ContainSubstring(tt.wantErr)))
				g.Expect(files).To(BeNil())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			want := make([]string, 0, len(tt.want))
			for _, name := range tt.want {
				want = append(want, filepath.Join(sourceDir, name))
			}
			g.Expect(files).To(Equal(want))
		})
	}

	t.Run("relative source directory produces absolute paths", func(t *testing.T) {
		g := NewWithT(t)
		t.Chdir(filepath.Dir(sourceDir))
		files, err := resolveFiles(filepath.Base(sourceDir), "a.yaml")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(files).To(Equal([]string{filepath.Join(sourceDir, "a.yaml")}))
	})
}
