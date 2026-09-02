package judge

import "os"

func writeFileForTest(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}

func osRead(path string) ([]byte, error) {
	return os.ReadFile(path)
}
