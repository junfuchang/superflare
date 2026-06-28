package define

import "github.com/junfuchang/superflare/config/data"

func dataOsGetwdForThemeTest() func() (string, error) {
	return data.TestGetwdExportForConfig()
}

func setDataOsGetwdForThemeTest(fn func() (string, error)) {
	data.TestSetGetwdExportForConfig(fn)
}

func restoreDataOsGetwdForThemeTest(fn func() (string, error)) {
	data.TestSetGetwdExportForConfig(fn)
}
