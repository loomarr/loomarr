package contact

import "testing"

func TestNormalize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		raw        string
		wantEmail  string
		wantKey    string
		shouldFail bool
	}{
		{name: "trim and fold", raw: "  Ada@Example.COM  ", wantEmail: "Ada@Example.COM", wantKey: "ada@example.com"},
		{name: "mailbox display name", raw: "Ada Lovelace <Ada@Example.COM>", wantEmail: "Ada@Example.COM", wantKey: "ada@example.com"},
		{name: "preserve plus addressing", raw: "ada+tv@example.com", wantEmail: "ada+tv@example.com", wantKey: "ada+tv@example.com"},
		{name: "empty", raw: "  ", shouldFail: true},
		{name: "multiple", raw: "ada@example.com, grace@example.com", shouldFail: true},
		{name: "invalid", raw: "not-an-email", shouldFail: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, key, err := Normalize(tt.raw)
			if tt.shouldFail {
				if err == nil {
					t.Fatalf("Normalize(%q) succeeded: %q, %q", tt.raw, email, key)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if email != tt.wantEmail || key != tt.wantKey {
				t.Fatalf("Normalize(%q) = %q, %q; want %q, %q", tt.raw, email, key, tt.wantEmail, tt.wantKey)
			}
		})
	}
}
