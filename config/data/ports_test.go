package data

import (
	"testing"

	"github.com/junfuchang/superflare/config/model"
)

func TestNormalizePortBindings(t *testing.T) {
	got := normalizePortBindings([]model.PortBinding{
		{Port: 3060, Protocol: "", Remark: " app "},
		{Port: 80, Protocol: "udp", Remark: "dns"},
		{Port: 1234, Protocol: "tcp", Hidden: true},
		{Port: 4321, Protocol: "tcp"},
		{Port: 0, Protocol: "tcp", Remark: "bad"},
		{Port: 3060, Protocol: "tcp", Remark: "new"},
	})
	if len(got) != 3 {
		t.Fatalf("expected 3 bindings, got %#v", got)
	}
	if got[0].Port != 80 || got[0].Protocol != "udp" || got[0].Remark != "dns" {
		t.Fatalf("unexpected first binding: %#v", got[0])
	}
	if got[1].Port != 1234 || got[1].Protocol != "tcp" || !got[1].Hidden {
		t.Fatalf("unexpected hidden binding: %#v", got[1])
	}
	if got[2].Port != 3060 || got[2].Protocol != "tcp" || got[2].Remark != "new" {
		t.Fatalf("unexpected third binding: %#v", got[2])
	}
}
