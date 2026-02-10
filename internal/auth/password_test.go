package auth

import "testing"

func TestNormalizeUsername(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "valid", raw: "Admin.User", want: "admin.user"},
		{name: "trim", raw: "  a-user  ", want: "a-user"},
		{name: "invalid chars", raw: "bad space", wantErr: true},
		{name: "empty", raw: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeUsername(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeUsername(%q)=%q want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("password-123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if !VerifyPassword(hash, "password-123") {
		t.Fatal("expected password to verify")
	}
	if VerifyPassword(hash, "wrong") {
		t.Fatal("expected wrong password to fail")
	}
}
