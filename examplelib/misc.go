package examplelib

import "os"

////////////////

func IsDir(name string) bool {
	fi, _ := os.Lstat(name)
	return fi != nil && fi.IsDirectory()
}

func IsFile(name string) bool {
	fi, _ := os.Lstat(name)
	return fi != nil && fi.IsRegular()
}

func FileExists(name string) bool {
	_, err := os.Lstat(name)
	return err == nil
}
