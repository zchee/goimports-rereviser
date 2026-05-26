package reviser_test

import (
	"testing"

	"github.com/zchee/goimports-rereviser/v4/reviser"
)

func TestFacadeExportsCompile(t *testing.T) {
	orders, err := reviser.StringToImportsOrders("std,general,company,project")
	if err != nil {
		t.Fatalf("StringToImportsOrders returned error: %v", err)
	}
	if len(orders) != 4 {
		t.Fatalf("expected 4 import groups, got %d", len(orders))
	}

	file := reviser.NewSourceFile("github.com/example/project", reviser.StandardInput)
	if err := reviser.WithImportsOrder(orders)(file); err != nil {
		t.Fatalf("WithImportsOrder returned error: %v", err)
	}

	if hash := reviser.ComputeContentHash([]byte("facade smoke")); hash == "" {
		t.Fatal("ComputeContentHash returned empty hash")
	}
}
