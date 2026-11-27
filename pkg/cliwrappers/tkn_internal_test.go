package cliwrappers

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestEncodeTknCSVValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain", input: "tasks/build.yaml", want: "tasks/build.yaml"},
		{name: "comma", input: "tasks/build,test.yaml", want: `"tasks/build,test.yaml"`},
		{name: "quotes", input: `tasks/build"release".yaml`, want: `"tasks/build""release"".yaml"`},
		{name: "newline", input: "tasks/build\ntest.yaml", want: "\"tasks/build\ntest.yaml\""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := encodeTknCSVValue(tt.input)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
