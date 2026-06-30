package define

import (
	"testing"

	"github.com/junfuchang/superflare/config/model"
)

func TestAppRuntimeFlagsSnapshotFallsBackInOrder(t *testing.T) {
	previous, wasSet := SnapshotAppRuntimeFlags()
	previousApp := AppFlags
	previousBase := AppBaseFlags
	previousSource := AppSourceFlags
	t.Cleanup(func() {
		AppFlags = previousApp
		AppBaseFlags = previousBase
		AppSourceFlags = previousSource
		if wasSet {
			StoreAppRuntimeFlags(previous.Source, previous.Base, previous.Current)
		} else {
			ResetAppRuntimeFlags()
		}
	})

	AppFlags = model.Flags{Port: 9000, User: "global-current"}
	AppBaseFlags = model.Flags{Port: 9001, User: "global-base"}
	AppSourceFlags = model.Flags{Port: 9002, User: "global-source"}
	ResetAppRuntimeFlags()

	if got := CurrentAppRuntimeFlags(); got.User != "global-current" {
		t.Fatalf("expected current fallback to AppFlags, got %#v", got)
	}
	if got := BaseAppRuntimeFlags(); got.User != "global-base" {
		t.Fatalf("expected base fallback to AppBaseFlags, got %#v", got)
	}
	if got := SourceAppRuntimeFlags(); got.User != "global-source" {
		t.Fatalf("expected source fallback to AppSourceFlags, got %#v", got)
	}

	StoreAppRuntimeFlags(
		model.Flags{},
		model.Flags{Port: 3636, User: "runtime-base"},
		model.Flags{Port: 3636, User: "runtime-current"},
	)

	if got := CurrentAppRuntimeFlags(); got.User != "runtime-current" {
		t.Fatalf("expected stored current flags, got %#v", got)
	}
	if got := BaseAppRuntimeFlags(); got.User != "runtime-base" {
		t.Fatalf("expected stored base flags, got %#v", got)
	}
	if got := SourceAppRuntimeFlags(); got.User != "runtime-base" {
		t.Fatalf("expected source fallback to stored base flags, got %#v", got)
	}
}
