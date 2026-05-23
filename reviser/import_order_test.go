package reviser

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestStringToImportsOrder(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		importsOrder string
		wantErr      string
	}{
		"invalid groupsImports count": {
			importsOrder: "std,general",
			wantErr:      `use default at least 4 parameters to sort groups of your imports: "std,general,company,project"`,
		},
		"unknown group": {
			importsOrder: "std,general,company,group",
			wantErr:      `unknown order group type: "group"`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got, err := StringToImportsOrders(tt.importsOrder)
			if got != nil {
				t.Errorf("expected nil, got: %v", got)
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("expected error %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestUnique_Deduplicates(t *testing.T) {
	t.Parallel()

	got := unique([]string{"a", "a", "b", "a", "c", "b"})
	want := []string{"a", "b", "c"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unique mismatch (-want +got):\n%s", diff)
	}
}

func TestStringToImportsOrders_IgnoresDuplicates(t *testing.T) {
	t.Parallel()

	gotDup, err := StringToImportsOrders("std,std,company,project,general")
	if err != nil {
		t.Fatalf("unexpected error from duplicated input: %v", err)
	}
	gotUniq, err := StringToImportsOrders("std,company,project,general")
	if err != nil {
		t.Fatalf("unexpected error from unique input: %v", err)
	}
	if diff := cmp.Diff(gotUniq, gotDup); diff != "" {
		t.Errorf("expected duplicated input to produce same order as unique input (-want +got):\n%s", diff)
	}
	if len(gotDup) != 4 {
		t.Errorf("expected 4 groups after dedup, got %d: %v", len(gotDup), gotDup)
	}
}

func TestAppendGroups(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		input [][]string
		want  []string
	}{
		"empty": {
			input: [][]string{},
			want:  []string{},
		},
		"single": {
			input: [][]string{{"a", "b", "c"}},
			want:  []string{"a", "b", "c"},
		},
		"multiple": {
			input: [][]string{{"a", "b", "c"}, {"d", "e", "f"}},
			want:  []string{"a", "b", "c", "\n", "\n", "d", "e", "f"},
		},
		"skip-empty": {
			input: [][]string{{"a", "b", "c"}, {}, {"d", "e", "f"}},
			want:  []string{"a", "b", "c", "\n", "\n", "d", "e", "f"},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := appendGroups(tt.input...)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("appendGroups() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
