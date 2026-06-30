package settings

import "testing"

func TestParseOptionalColor(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		field   string
		want    string
		wantErr string
	}{
		{name: "empty", input: "", field: "bookmark", want: ""},
		{name: "valid", input: "rgba(1, 2, 3, 0.5)", field: "bookmark", want: "rgba(1, 2, 3, 0.5)"},
		{name: "invalid generic", input: "bad", field: "", wantErr: "invalid color value: bad"},
		{name: "invalid fielded", input: "bad", field: "custom-theme-primary", wantErr: "invalid custom-theme-primary value: bad"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseOptionalColor(tc.input, tc.field)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("expected error %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseOptionalRangedInt(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		min     int
		max     int
		field   string
		want    int
		wantErr string
	}{
		{name: "empty", input: "", min: 0, max: 8, field: "home-max-columns", want: 0},
		{name: "valid", input: "6", min: 0, max: 8, field: "home-max-columns", want: 6},
		{name: "nan", input: "abc", min: 0, max: 8, field: "home-max-columns", wantErr: "invalid home-max-columns value: abc"},
		{name: "out of range", input: "9", min: 0, max: 8, field: "home-max-columns", wantErr: "home-max-columns must be between 0 and 8"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseOptionalRangedInt(tc.input, tc.min, tc.max, tc.field)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("expected error %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}
