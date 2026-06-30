package define

import (
	"sync"

	"github.com/junfuchang/superflare/config/model"
)

type AppRuntimeFlagsSnapshot struct {
	Source  model.Flags
	Base    model.Flags
	Current model.Flags
}

type appRuntimeFlagsHolder struct {
	mu  sync.RWMutex
	set bool
	cfg AppRuntimeFlagsSnapshot
}

var appRuntimeFlags = &appRuntimeFlagsHolder{}

func StoreAppRuntimeFlags(source model.Flags, base model.Flags, current model.Flags) {
	appRuntimeFlags.mu.Lock()
	appRuntimeFlags.cfg = AppRuntimeFlagsSnapshot{
		Source:  source,
		Base:    base,
		Current: current,
	}
	appRuntimeFlags.set = true
	appRuntimeFlags.mu.Unlock()
	MirrorAppRuntimeFlagsForLegacyGlobals(source, base, current)
}

// MirrorAppRuntimeFlagsForLegacyGlobals keeps the historical public variables
// synchronized for old callers and tests. Runtime code should use
// SnapshotAppRuntimeFlags/CurrentAppRuntimeFlags instead of reading these
// compatibility globals directly.
func MirrorAppRuntimeFlagsForLegacyGlobals(source model.Flags, base model.Flags, current model.Flags) {
	AppSourceFlags = source
	AppBaseFlags = base
	AppFlags = current
}

func StoreAppRuntimeCurrentFlags(flags model.Flags) {
	StoreAppRuntimeFlags(flags, flags, flags)
}

func SnapshotAppRuntimeFlags() (AppRuntimeFlagsSnapshot, bool) {
	appRuntimeFlags.mu.RLock()
	defer appRuntimeFlags.mu.RUnlock()
	return appRuntimeFlags.cfg, appRuntimeFlags.set
}

func CurrentAppRuntimeFlags() model.Flags {
	if cfg, ok := SnapshotAppRuntimeFlags(); ok {
		return cfg.Current
	}
	return AppFlags
}

func BaseAppRuntimeFlags() model.Flags {
	if cfg, ok := SnapshotAppRuntimeFlags(); ok {
		if cfg.Base.Port != 0 {
			return cfg.Base
		}
		return cfg.Current
	}
	if AppBaseFlags.Port != 0 {
		return AppBaseFlags
	}
	return AppFlags
}

func SourceAppRuntimeFlags() model.Flags {
	if cfg, ok := SnapshotAppRuntimeFlags(); ok {
		if cfg.Source.Port != 0 {
			return cfg.Source
		}
		if cfg.Base.Port != 0 {
			return cfg.Base
		}
		return cfg.Current
	}
	if AppSourceFlags.Port != 0 {
		return AppSourceFlags
	}
	return BaseAppRuntimeFlags()
}

func ResetAppRuntimeFlags() {
	appRuntimeFlags.mu.Lock()
	appRuntimeFlags.cfg = AppRuntimeFlagsSnapshot{}
	appRuntimeFlags.set = false
	appRuntimeFlags.mu.Unlock()
}
