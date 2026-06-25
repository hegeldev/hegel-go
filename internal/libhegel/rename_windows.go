package libhegel

import (
	"os"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fileRenameInfoEx mirrors the Win32 FILE_RENAME_INFO structure as interpreted
// by the FileRenameInfoEx information class, whose leading union member is the
// Flags bitmask (rather than the BOOLEAN ReplaceIfExists of the legacy
// FileRenameInfo class). FileName is an inline trailing array; a fixed
// MAX_PATH backing keeps the struct a plain value, matching the Go runtime's
// own renameat implementation.
type fileRenameInfoEx struct {
	Flags          uint32
	RootDirectory  windows.Handle
	FileNameLength uint32
	FileName       [syscall.MAX_PATH]uint16
}

// atomicRename replaces newpath with oldpath using POSIX rename semantics.
//
// A plain MoveFileEx (what os.Rename uses) fails with ERROR_ACCESS_DENIED when
// the destination is a DLL another process has loaded via LoadLibrary, because
// the loaded image is mapped without FILE_SHARE_DELETE. SetFileInformationByHandle
// with FileRenameInfoEx and FILE_RENAME_POSIX_SEMANTICS instead unlinks the old
// destination immediately and defers its teardown until the last handle closes —
// exactly how rename(2) behaves on Unix — so a concurrently-loaded library no
// longer blocks the replace.
func atomicRename(oldpath, newpath string) error {
	wrap := func(err error) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: err}
	}

	from, err := windows.UTF16PtrFromString(oldpath)
	if err != nil {
		return wrap(err)
	}
	// DELETE access is what SetFileInformationByHandle requires to rename, and
	// the full share mode lets other handles to the source coexist.
	h, err := windows.CreateFile(
		from,
		windows.DELETE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return wrap(err)
	}
	defer windows.CloseHandle(h)

	to, err := windows.UTF16FromString(newpath)
	if err != nil {
		return wrap(err)
	}

	info := fileRenameInfoEx{
		Flags: windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS,
	}
	if len(to) > len(info.FileName) {
		return wrap(syscall.ENAMETOOLONG)
	}
	copy(info.FileName[:], to)
	// FileNameLength is in bytes and excludes the terminating NUL that
	// UTF16FromString appends.
	info.FileNameLength = uint32((len(to) - 1) * 2)

	if err := windows.SetFileInformationByHandle(
		h,
		windows.FileRenameInfoEx,
		(*byte)(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		return wrap(err)
	}
	return nil
}
