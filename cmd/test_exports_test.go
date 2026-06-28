package cmd

func ResolveDotEnvPathForTest() func() (string, error) {
	return resolveDotEnvPath
}

func SetResolveDotEnvPathForTest(next func() (string, error)) {
	resolveDotEnvPath = next
}
