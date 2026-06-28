package fn

func TestGetwdExportForAssets() func() (string, error) {
	return testGetwdExport()
}

func TestSetGetwdExportForAssets(next func() (string, error)) {
	testSetGetwdExport(next)
}
