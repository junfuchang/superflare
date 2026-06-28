package data

func TestGetwdExportForConfig() func() (string, error) {
	return osGetwd
}

func TestSetGetwdExportForConfig(fn func() (string, error)) {
	osGetwd = fn
}
