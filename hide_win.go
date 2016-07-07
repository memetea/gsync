// +build windows

package gsync

import "syscall"

func HideFile(path string) error {
	pname := syscall.StringToUTF16Ptr(path)
	attrs, err := syscall.GetFileAttributes(pname)
	if err != nil {
		return err
	}
	attrs = attrs | syscall.FILE_ATTRIBUTE_HIDDEN
	return syscall.SetFileAttributes(pname, attrs)
}

func UnHideFile(path string) error {
	pname := syscall.StringToUTF16Ptr(path)
	attrs, err := syscall.GetFileAttributes(pname)
	if err != nil {
		return err
	}
	attrs = attrs & ^uint32(syscall.FILE_ATTRIBUTE_HIDDEN)
	return syscall.SetFileAttributes(pname, attrs)
}
