package reviser_test

import (
	"testing"

	"github.com/zchee/goimports-rereviser/v4/reviser"
)

func TestFacadeExportsCompile(t *testing.T) {
	orders, err := reviser.StringToImportsOrders("std,general,company,project,nonblank")
	if err != nil {
		t.Fatalf("StringToImportsOrders returned error: %v", err)
	}
	if len(orders) != 5 {
		t.Fatalf("expected 5 import groups, got %d", len(orders))
	}
	if orders[4] != reviser.NonBlankImportsOrder {
		t.Fatalf("expected nonblank order at index 4, got %q", orders[4])
	}

	file := reviser.NewSourceFile("github.com/example/project", reviser.StandardInput)
	if err := reviser.WithImportsOrder(orders)(file); err != nil {
		t.Fatalf("WithImportsOrder returned error: %v", err)
	}

	if hash := reviser.ComputeContentHash([]byte("facade smoke")); hash == "" {
		t.Fatal("ComputeContentHash returned empty hash")
	}
}
