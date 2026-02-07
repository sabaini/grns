package server

import "testing"

func TestValidateID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"gr-ab12", true},
		{"zz-0000", true},
		{"gr-zzzz", true},
		{"", false},
		{"gr", false},
		{"gr-", false},
		{"gr-abc", false},   // too short
		{"gr-abcde", false}, // too long
		{"GR-ab12", false},  // uppercase prefix
		{"gr-AB12", false},  // uppercase hash
		{"gr_ab12", false},  // wrong separator
		{"abc-ab12", false}, // 3-letter prefix
		{"g-ab12", false},   // 1-letter prefix
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := validateID(tt.id)
			if got != tt.want {
				t.Fatalf("validateID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestNormalizeStatus(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"open", "open", false},
		{"OPEN", "open", false},
		{"in_progress", "in_progress", false},
		{"closed", "closed", false},
		{"tombstone", "tombstone", false},
		{"pinned", "pinned", false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := normalizeStatus(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeStatus(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("normalizeStatus(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeType(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"bug", "bug", false},
		{"BUG", "bug", false},
		{"feature", "feature", false},
		{"task", "task", false},
		{"epic", "epic", false},
		{"chore", "chore", false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := normalizeType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeType(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("normalizeType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeLabel(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"bug", "bug", false},
		{"BUG", "bug", false},
		{"critical", "critical", false},
		{"a-b", "a-b", false},
		{"", "", true},
		{"hello world", "", true}, // contains space
		{"café", "", true},        // non-ASCII
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := normalizeLabel(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeLabel(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("normalizeLabel(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizePrefix(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"gr", "gr", false},
		{"GR", "gr", false},
		{"ab", "ab", false},
		{"a", "", true},   // too short
		{"abc", "", true}, // too long
		{"12", "", true},  // digits
		{"a1", "", true},  // mixed
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := normalizePrefix(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizePrefix(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("normalizePrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
