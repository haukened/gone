package integration_test

import (
	"strings"
	"testing"
)

// TestLoadSpecSelf checks the extractor against known entries of the spec.
func TestLoadSpecSelf(t *testing.T) {
	ops := loadSpec(t)
	find := func(method, path string) *specOp {
		for i := range ops {
			if ops[i].method == method && ops[i].path == path {
				return &ops[i]
			}
		}
		t.Fatalf("%s %s not found", method, path)
		return nil
	}
	create := find("POST", "/api/secret")
	if _, ok := create.codes["201"]; !ok || len(create.codes) < 6 {
		t.Fatalf("create codes = %v", create.codes)
	}
	if !strings.Contains(create.codes["503"], "Retry-After") {
		t.Fatalf("503 block does not expand the Busy component: %q", create.codes["503"])
	}
	if got := find("GET", "/readyz").codes["503"]; !strings.Contains(got, "application/json") {
		t.Fatalf("readyz 503 = %q", got)
	}
	if find("PUT", "/api/request/{id}/reply") == nil || find("GET", "/i18n/{tag}.json") == nil {
		t.Fatal("missing operations")
	}
}
