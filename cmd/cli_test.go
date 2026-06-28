package cmd_test

import (
	"os"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/cmd"
	"github.com/junfuchang/superflare/config/define"
	"github.com/stretchr/testify/assert"
)

func TestGetCliFlags(t *testing.T) {
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	tests := []struct {
		name     string
		args     []string
		wantPort int
	}{
		{
			name:     "empty args",
			args:     nil,
			wantPort: define.DEFAULT_PORT,
		},
		{
			name:     "set port",
			args:     []string{"--port", "9090"},
			wantPort: 9090,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = append([]string{"app"}, tt.args...)
			gotFlags, _, err := cmd.GetCliFlags()
			if err != nil {
				t.Fatalf("GetCliFlags: %v", err)
			}
			assert.Equal(t, tt.wantPort, gotFlags.Port)
			assert.Equal(t, define.DEFAULT_ENABLE_GUIDE, gotFlags.EnableGuide)
		})
	}
}

func TestGetCliFlagsReturnsErrorForInvalidArguments(t *testing.T) {
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	os.Args = []string{"app", "--port=bad"}
	_, _, err := cmd.GetCliFlags()
	if err == nil {
		t.Fatal("expected GetCliFlags to fail")
	}
	if !strings.Contains(err.Error(), "parse cli flags failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetFlagsMaps(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	tests := []struct {
		name string
		args []string
		want map[string]bool
	}{
		{
			name: "test single dash flags",
			args: []string{"cmd", "-foo", "-bar=value", "-baz"},
			want: map[string]bool{"foo": true, "bar": true, "baz": true},
		},
		{
			name: "test double dash flags",
			args: []string{"cmd", "--alpha", "--beta=ok", "--gamma"},
			want: map[string]bool{"alpha": true, "beta": true, "gamma": true},
		},
		{
			name: "test mixed dash flags",
			args: []string{"cmd", "--apple", "-banana=yellow", "--cherry", "-date"},
			want: map[string]bool{"apple": true, "banana": true, "cherry": true, "date": true},
		},
		{
			name: "test no flags",
			args: []string{"cmd"},
			want: map[string]bool{},
		},
		{
			name: "test empty args",
			args: []string{},
			want: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Args = tt.args
			got := cmd.GetFlagsMaps()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCheckFlagsExists(t *testing.T) {
	tests := []struct {
		name   string
		dict   map[string]bool
		keys   []string
		expect bool
	}{
		{
			name:   "all false",
			dict:   map[string]bool{"a": false, "b": false, "c": false},
			keys:   []string{"a", "b"},
			expect: false,
		},
		{
			name:   "one true",
			dict:   map[string]bool{"a": true, "b": false, "c": false},
			keys:   []string{"a", "b"},
			expect: true,
		},
		{
			name:   "none existent",
			dict:   map[string]bool{"a": true, "b": true},
			keys:   []string{"c", "d"},
			expect: false,
		},
		{
			name:   "empty keys",
			dict:   map[string]bool{"a": true, "b": true},
			keys:   []string{},
			expect: false,
		},
		{
			name:   "empty dict",
			dict:   map[string]bool{},
			keys:   []string{"a", "b"},
			expect: false,
		},
		{
			name:   "nil dict",
			dict:   nil,
			keys:   []string{"a", "b"},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cmd.CheckFlagsExists(tt.dict, tt.keys)
			assert.Equal(t, result, tt.expect)
		})
	}
}
