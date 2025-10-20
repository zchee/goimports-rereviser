package reviser

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestStringToImportsOrder(t *testing.T) {
	t.Parallel()

	type args struct {
		importsOrder string
	}

	tests := []struct {
		name    string
		args    args
		wantErr string
	}{
		{
			name:    "invalid groupsImports count",
			args:    args{importsOrder: "std,general"},
			wantErr: `use default at least 4 parameters to sort groups of your imports: "std,general,company,project"`,
		},
		{
			name:    "unknown group",
			args:    args{importsOrder: "std,general,company,group"},
			wantErr: `unknown order group type: "group"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := StringToImportsOrders(tt.args.importsOrder)

			if got != nil {
				t.Errorf("expected nil, got: %v", got)
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("expected error %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func Test_appendGroups(t *testing.T) {
	type args struct {
		input [][]string
	}
	tests := []struct {
		name string
		args args
		want []string
	}{
		{
			name: "empty",
			args: args{input: [][]string{}},
			want: []string{},
		},
		{
			name: "single",
			args: args{input: [][]string{{"a", "b", "c"}}},
			want: []string{"a", "b", "c"},
		},
		{
			name: "multiple",
			args: args{input: [][]string{{"a", "b", "c"}, {"d", "e", "f"}}},
			want: []string{"a", "b", "c", "\n", "\n", "d", "e", "f"},
		},
		{
			name: "skip-empty",
			args: args{input: [][]string{{"a", "b", "c"}, {}, {"d", "e", "f"}}},
			want: []string{"a", "b", "c", "\n", "\n", "d", "e", "f"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := appendGroups(tt.args.input...)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("appendGroups() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
