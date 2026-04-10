package verify

import (
	"os"
)

func t_tempFile(content string) string {
	f, err := os.CreateTemp("", "shen-derive-verify-*.shen")
	if err != nil {
		panic(err)
	}
	if _, err := f.WriteString(content); err != nil {
		panic(err)
	}
	f.Close()
	return f.Name()
}

func t_removeFile(path string) {
	_ = os.Remove(path)
}
