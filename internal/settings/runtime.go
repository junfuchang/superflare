package settings

import (
	"sync"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
)

type runtimeSnapshot struct {
	DebugMode bool
}

type runtimeHolder struct {
	mu  sync.RWMutex
	set bool
	cfg runtimeSnapshot
}

func (h *runtimeHolder) Load() runtimeSnapshot {
	if h == nil {
		return runtimeSnapshot{}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.set {
		return runtimeSnapshot{}
	}
	return h.cfg
}

func (h *runtimeHolder) Store(cfg runtimeSnapshot) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.set = true
	h.cfg = cfg
	h.mu.Unlock()
}

var settingsRuntimeFlags = &runtimeHolder{}

func settingsRuntimeSnapshotFromFlags(flags model.Flags) runtimeSnapshot {
	return runtimeSnapshot{DebugMode: flags.DebugMode}
}

func CurrentRuntime() runtimeSnapshot {
	settingsRuntimeFlags.mu.RLock()
	hasValue := settingsRuntimeFlags.set
	cfg := settingsRuntimeFlags.cfg
	settingsRuntimeFlags.mu.RUnlock()
	if hasValue {
		return cfg
	}
	cfg = settingsRuntimeSnapshotFromFlags(define.CurrentAppRuntimeFlags())
	settingsRuntimeFlags.Store(cfg)
	return cfg
}

func SetRuntimeFlags(flags model.Flags) {
	settingsRuntimeFlags.Store(settingsRuntimeSnapshotFromFlags(flags))
}
