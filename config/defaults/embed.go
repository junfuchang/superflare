package defaults

import "embed"

// Files exposes the repository-tracked runtime defaults used by first-run init.
//
//go:embed *.yml *.yaml
var Files embed.FS
