package domain

import "testing"

func TestParseID(t *testing.T) {
	valid, err := ParseID("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid.Valid() {
		t.Fatalf("Valid() returned false for a valid id")
	}

	cases := []string{"", "short", "XYZ", "0123456789ABCDEF0123456789ABCDEF", "0123456789abcdef0123456789abcdeg"}
	for _, c := range cases {
		if _, err := ParseID(c); err == nil {
			t.Errorf("expected error for %q", c)
		}
	}
}

func TestNewID(t *testing.T) {
	const n = 10
	unique := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id, err := NewID()
		if err != nil {
			t.Fatalf("NewID error: %v", err)
		}
		s := id.String()
		if len(s) != 32 {
			t.Fatalf("id length unexpected: %d", len(s))
		}
		if !id.Valid() {
			t.Fatalf("generated id invalid: %s", id)
		}
		// Ensure all characters are lowercase hex explicitly.
		for _, c := range s {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				t.Fatalf("id contains non-hex lowercase char: %s", s)
			}
		}
		if _, exists := unique[s]; exists {
			t.Fatalf("duplicate id generated: %s", s)
		}
		unique[s] = struct{}{}
	}
	if len(unique) != n { // extremely unlikely; indicates collision or logic error
		t.Fatalf("expected %d unique ids, got %d", n, len(unique))
	}
}

func TestSecretIDValidMethod(t *testing.T) {
	id := SecretID("0123456789abcdef0123456789abcdef")
	if !id.Valid() {
		t.Fatalf("expected id to be valid")
	}
	bad := SecretID("g123456789abcdef0123456789abcdef")
	if bad.Valid() {
		t.Fatalf("expected invalid id")
	}
}
