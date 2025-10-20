package helper

import (
	"errors"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/zchee/goimports-rereviser/v4/reviser"
)

func TestDetermineProjectName(t *testing.T) {
	t.Parallel()

	type args struct {
		projectName string
		filePath    string
		option      Option
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "success with manual filepath",
			args: args{
				projectName: "",
				filePath: func() string {
					dir, err := os.Getwd()
					if err != nil {
						t.Fatalf("unexpected error: %v", err)
					}
					return dir
				}(),
				option: OSGetwdOption,
			},
			want: "github.com/zchee/goimports-rereviser/v4",
		},
		{
			name: "success with stdin",
			args: args{
				projectName: "",
				filePath:    reviser.StandardInput,
				option:      OSGetwdOption,
			},
			want: "github.com/zchee/goimports-rereviser/v4",
		},
		{
			name: "fail with manual filepath",
			args: args{
				projectName: "",
				filePath:    reviser.StandardInput,
				option: func() (string, error) {
					return "", errors.New("some error")
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := DetermineProjectName(tt.args.projectName, tt.args.filePath, tt.args.option)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
