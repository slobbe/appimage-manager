package filesystem

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"
)

func Sha256AndSha1(path string) (string, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	h256 := sha256.New()
	h1 := sha1.New()

	if _, err := io.Copy(io.MultiWriter(h256, h1), f); err != nil {
		return "", "", err
	}

	return hex.EncodeToString(h256.Sum(nil)), hex.EncodeToString(h1.Sum(nil)), nil
}

func Sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func Sha1(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func VerifyHashes(path, expectedSHA256, expectedSHA1 string) error {
	expectedSHA256 = strings.ToLower(strings.TrimSpace(expectedSHA256))
	expectedSHA1 = strings.ToLower(strings.TrimSpace(expectedSHA1))

	if expectedSHA256 != "" && expectedSHA1 != "" {
		sha256sum, sha1sum, err := Sha256AndSha1(path)
		if err != nil {
			return err
		}
		if strings.ToLower(sha256sum) != expectedSHA256 {
			return fmt.Errorf("downloaded file sha256 mismatch")
		}
		if strings.ToLower(sha1sum) != expectedSHA1 {
			return fmt.Errorf("downloaded file sha1 mismatch")
		}
		return nil
	}

	if expectedSHA256 != "" {
		sum, err := Sha256File(path)
		if err != nil {
			return err
		}
		if strings.ToLower(sum) != expectedSHA256 {
			return fmt.Errorf("downloaded file sha256 mismatch")
		}
	}

	if expectedSHA1 != "" {
		sum, err := Sha1(path)
		if err != nil {
			return err
		}
		if strings.ToLower(sum) != expectedSHA1 {
			return fmt.Errorf("downloaded file sha1 mismatch")
		}
	}

	return nil
}
