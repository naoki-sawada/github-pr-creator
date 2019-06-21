package utils

import (
	"encoding/base64"
	"os"
)

func Base64ToFile(b64 string, filename string) error {
	dec, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return err
	}

	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(dec); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}

	return nil
}
