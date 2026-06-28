package assets

import "github.com/junfuchang/superflare/internal/fn"

func fnGetwdForAssetsTest() func() (string, error) {
	return fn.TestGetwdExportForAssets()
}

func setFnGetwdForAssetsTest(next func() (string, error)) {
	fn.TestSetGetwdExportForAssets(next)
}

func restoreFnGetwdForAssetsTest(original func() (string, error)) {
	fn.TestSetGetwdExportForAssets(original)
}
