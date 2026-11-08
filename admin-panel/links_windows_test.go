package main

import (
	"os"
	"syscall"
	"testing"
)

func TestWindowsReplaceFailurePreservesConfig(t *testing.T) {
	a := testApp(t)
	before, e := os.ReadFile(a.store.path)
	if e != nil {
		t.Fatal(e)
	}
	rev := currentRevision(t, a)
	name, e := syscall.UTF16PtrFromString(a.store.path)
	if e != nil {
		t.Fatal(e)
	}
	// Deny delete-sharing to simulate another Windows program locking the existing file.
	handle, e := syscall.CreateFile(name, syscall.GENERIC_READ, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, 0, 0)
	if e != nil {
		t.Fatal(e)
	}
	defer syscall.CloseHandle(handle)
	if e = a.store.update("fivesim", "https://5sim.net/?fixture=locked", rev, false); e == nil {
		t.Fatal("replace succeeded despite file lock")
	}
	after, e := os.ReadFile(a.store.path)
	if e != nil {
		t.Fatal(e)
	}
	if string(before) != string(after) {
		t.Fatal("failed replacement changed existing file")
	}
}
