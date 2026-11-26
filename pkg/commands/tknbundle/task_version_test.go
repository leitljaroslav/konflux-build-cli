package tknbundle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
)

func TestGetTaskVersion(t *testing.T) {
	const taskManifest = `apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: build
  labels:
    app: example
    app.kubernetes.io/version: "0.2.5"
  annotations:
    description: Build the image
spec:
  steps:
    - name: build
      image: quay.io/example/tools:v1
      script: echo build
`

	tests := []struct {
		name    string
		task    string
		want    string
		wantErr bool
		missing bool
	}{
		{
			name:    "parse the version correctly",
			task:    taskManifest,
			want:    "0.2.5",
			wantErr: false,
		},
		{
			name: "surrounding whitespace",
			task: strings.Replace(taskManifest, `"0.2.5"`, `" 0.2.5 "`, 1),
			want: "0.2.5",
		},
		{
			name: "spaces tabs and newlines throughout version",
			task: strings.Replace(taskManifest, `"0.2.5"`, `" 0 .2.\t5\n"`, 1),
			want: "0.2.5",
		},
		{
			name: "missing version label",
			task: strings.Replace(taskManifest, "    app.kubernetes.io/version: \"0.2.5\"\n", "", 1),
		},
		{name: "missing metadata", task: "apiVersion: tekton.dev/v1\nkind: Task\n"},
		{name: "missing labels", task: "metadata:\n  name: build\n"},
		{
			name: "null version label",
			task: strings.Replace(taskManifest, `"0.2.5"`, "null", 1),
		},
		{
			name: "empty version label",
			task: strings.Replace(taskManifest, `"0.2.5"`, `""`, 1),
		},
		{
			name: "whitespace only version label",
			task: strings.Replace(taskManifest, `"0.2.5"`, `" \t\n "`, 1),
		},
		{
			name: "multiple documents retain separators",
			task: taskManifest + "---\n" + strings.Replace(taskManifest, `"0.2.5"`, `"1.0"`, 1),
			want: "0.2.5---1.0",
		},
		{
			name: "missing version in later document retains separator",
			task: taskManifest + "---\nkind: Pipeline\n",
			want: "0.2.5---",
		},
		{
			name: "lines containing null are removed",
			task: strings.Replace(taskManifest, `"0.2.5"`, `"0.\nnullable\n2.5"`, 1),
			want: "0.2.5",
		},
		{name: "empty file"},
		{name: "malformed YAML", task: "metadata: [", wantErr: true},
		{name: "malformed later YAML document", task: taskManifest + "---\nmetadata: [", wantErr: true},
		{name: "missing file", missing: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			tmpTaskFileName := filepath.Join(t.TempDir(), "task.yaml")
			if !tt.missing {
				g.Expect(os.WriteFile(tmpTaskFileName, []byte(tt.task), 0o600)).To(Succeed())
			}

			version, err := getTaskVersion(tmpTaskFileName)

			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(version).To(Equal(tt.want))
		})
	}
}
