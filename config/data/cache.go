package data

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

type fileCacheEntry struct {
	data    []byte
	size    int64
	modTime time.Time
}

var (
	fileCacheMu sync.RWMutex
	fileCache   = map[string]fileCacheEntry{}
)

func readFileCached(filePath string, readDisk func() ([]byte, error)) ([]byte, error) {
	cacheKey := filepath.Clean(filePath)
	stat, statErr := os.Stat(cacheKey)

	fileCacheMu.RLock()
	if cached, ok := fileCache[cacheKey]; ok && cacheEntryMatchesStat(cached, stat, statErr) {
		fileCacheMu.RUnlock()
		return cloneBytes(cached.data), nil
	}
	fileCacheMu.RUnlock()

	fileCacheMu.Lock()
	defer fileCacheMu.Unlock()
	stat, statErr = os.Stat(cacheKey)
	if cached, ok := fileCache[cacheKey]; ok && cacheEntryMatchesStat(cached, stat, statErr) {
		return cloneBytes(cached.data), nil
	}

	raw, err := readDisk()
	if err != nil {
		return nil, err
	}
	fileCache[cacheKey] = newFileCacheEntry(raw, stat, statErr)
	return cloneBytes(raw), nil
}

func invalidateFileCachePath(filePath string) {
	fileCacheMu.Lock()
	defer fileCacheMu.Unlock()
	delete(fileCache, filepath.Clean(filePath))
}

func invalidateFileCache(name string) {
	switch name {
	case "config", "apps", "bookmarks":
		invalidateFileCachePath(getConfigPath(name))
	case "ports":
		invalidateFileCachePath(getPortsConfigPath())
	}
}

func InvalidateConfigCache(name string) {
	invalidateFileCache(name)
}

func cloneBytes(in []byte) []byte {
	if len(in) == 0 {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

func cacheEntryMatchesStat(entry fileCacheEntry, stat os.FileInfo, statErr error) bool {
	if statErr != nil {
		return false
	}
	return entry.size == stat.Size() && entry.modTime.Equal(stat.ModTime())
}

func newFileCacheEntry(raw []byte, stat os.FileInfo, statErr error) fileCacheEntry {
	entry := fileCacheEntry{
		data: cloneBytes(raw),
	}
	if statErr == nil && stat != nil {
		entry.size = stat.Size()
		entry.modTime = stat.ModTime()
	}
	return entry
}
