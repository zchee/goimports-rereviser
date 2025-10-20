package module

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPathIsNotSetError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{
			name: "success",
			want: "path is not set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &PathIsNotSetError{}
			got := e.Error()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUndefinedModuleError_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{
			name: "success",
			want: "module is undefined",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &UndefinedModuleError{}
			got := e.Error()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
