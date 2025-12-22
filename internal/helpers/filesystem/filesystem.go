package file

import (
	"io"
	"os"
)

func Move(file string, dest string) (string, error) {
	if err := os.Rename(file, dest); err != nil {
		return file, err
	}

	return dest, nil
}

func Copy(src, dst string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return src, err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return src, err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return dst, err
}

func MakeExecutable(src string) error {
	const execPerm = 0755
	if err := os.Chmod(src, execPerm); err != nil {
			return err
	}
	
	return nil
}
