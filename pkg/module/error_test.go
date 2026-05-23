package module

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestPathIsNotSetError_Error(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		want string
	}{
		"success": {
			want: "path is not set",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			e := &PathIsNotSetError{}
			if diff := cmp.Diff(tt.want, e.Error()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestUndefinedModuleError_Error(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		want string
	}{
		"success": {
			want: "module is undefined",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			e := &UndefinedModuleError{}
			if diff := cmp.Diff(tt.want, e.Error()); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
