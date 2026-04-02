package hegel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheDirFromXDG(t *testing.T) {
	t.Parallel()
	result := cacheDirFrom("/tmp/xdg", "")
	expected := filepath.Join("/tmp/xdg", "hegel")
	if result != expected {
		t.Errorf("cacheDirFrom with XDG = %q, want %q", result, expected)
	}
}

func TestCacheDirFromHome(t *testing.T) {
	t.Parallel()
	result := cacheDirFrom("", "/home/test")
	expected := filepath.Join("/home/test", ".cache", "hegel")
	if result != expected {
		t.Errorf("cacheDirFrom with home = %q, want %q", result, expected)
	}
}

func TestCacheDirFromXDGTakesPrecedence(t *testing.T) {
	t.Parallel()
	result := cacheDirFrom("/tmp/xdg", "/home/test")
	expected := filepath.Join("/tmp/xdg", "hegel")
	if result != expected {
		t.Errorf("cacheDirFrom should prefer XDG, got %q, want %q", result, expected)
	}
}

func TestFindUVImplUsesPathUV(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	fakeUV := filepath.Join(tmp, "uv")
	os.WriteFile(fakeUV, []byte("#!/bin/sh\n"), 0o755) //nolint:errcheck

	result, err := findUVImpl(fakeUV, "/nonexistent")
	if err != nil {
		t.Fatalf("findUVImpl: %v", err)
	}
	if result != fakeUV {
		t.Errorf("findUVImpl with path uv = %q, want %q", result, fakeUV)
	}
}

func TestFindUVImplReturnsCachedWhenNotInPath(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cachedUV := filepath.Join(tmp, "uv")
	os.WriteFile(cachedUV, []byte("#!/bin/sh\n"), 0o755) //nolint:errcheck

	result, err := findUVImpl("", tmp)
	if err != nil {
		t.Fatalf("findUVImpl: %v", err)
	}
	if result != cachedUV {
		t.Errorf("findUVImpl with cached = %q, want %q", result, cachedUV)
	}
}

func TestInstallUVFailsWithBadShell(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	err := installUVWithSh(tmp, "definitely_not_a_real_shell_xyz")
	if err == nil {
		t.Fatal("expected error from installUVWithSh with bad shell")
	}
}

func TestInstallUVMkdirFails(t *testing.T) {
	t.Parallel()
	// Use a file path as the cache dir so MkdirAll fails.
	tmp := t.TempDir()
	blocker := filepath.Join(tmp, "blocker")
	os.WriteFile(blocker, []byte("not a dir"), 0o644) //nolint:errcheck
	err := installUVWithSh(filepath.Join(blocker, "subdir"), "sh")
	if err == nil {
		t.Fatal("expected error when mkdir fails")
	}
}

func TestInstallUVSuccess(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	cacheDir := filepath.Join(tmp, "cache")

	// Create a fake shell that writes a "uv" binary to the UV_UNMANAGED_INSTALL dir.
	scriptDir := t.TempDir()
	fakeShPath := filepath.Join(scriptDir, "fake_sh")
	fakeShScript := `#!/bin/sh
# Read and discard stdin (the installer script)
cat > /dev/null
# Create a fake uv binary in the install dir
touch "$UV_UNMANAGED_INSTALL/uv"
chmod +x "$UV_UNMANAGED_INSTALL/uv"
`
	os.WriteFile(fakeShPath, []byte(fakeShScript), 0o755) //nolint:errcheck

	err := installUVWithSh(cacheDir, fakeShPath)
	if err != nil {
		t.Fatalf("installUVWithSh: %v", err)
	}

	// Verify the uv binary was created.
	uvPath := filepath.Join(cacheDir, "uv")
	if _, err := os.Stat(uvPath); err != nil {
		t.Errorf("expected uv binary at %s, got error: %v", uvPath, err)
	}
}

func TestFindUVImplInstallsAndReturnsCached(t *testing.T) {
	tmp := t.TempDir()
	cacheDir := filepath.Join(tmp, "cache")

	// Mock installUVFn to create a fake uv binary.
	origInstall := installUVFn
	installUVFn = func(dir string) error {
		os.MkdirAll(dir, 0o755) //nolint:errcheck
		return os.WriteFile(filepath.Join(dir, "uv"), []byte("#!/bin/sh\n"), 0o755)
	}
	defer func() { installUVFn = origInstall }()

	result, err := findUVImpl("", cacheDir)
	if err != nil {
		t.Fatalf("findUVImpl: %v", err)
	}
	expected := filepath.Join(cacheDir, "uv")
	if result != expected {
		t.Errorf("findUVImpl = %q, want %q", result, expected)
	}
}

func TestFindUVImplMkdirCacheFails(t *testing.T) {
	tmp := t.TempDir()
	// Block cacheDir creation by placing a file where the directory should be.
	blocker := filepath.Join(tmp, "blocker")
	os.WriteFile(blocker, []byte("not a dir"), 0o644) //nolint:errcheck
	cacheDir := filepath.Join(blocker, "cache")

	origInstall := installUVFn
	installUVFn = func(dir string) error {
		return os.WriteFile(filepath.Join(dir, "uv"), []byte("#!/bin/sh\n"), 0o755)
	}
	defer func() { installUVFn = origInstall }()

	_, err := findUVImpl("", cacheDir)
	if err == nil {
		t.Fatal("expected error when cacheDir creation fails")
	}
}

func TestFindUVImplInstallFails(t *testing.T) {
	tmp := t.TempDir()
	cacheDir := filepath.Join(tmp, "cache")

	origInstall := installUVFn
	installUVFn = func(dir string) error {
		return os.ErrPermission
	}
	defer func() { installUVFn = origInstall }()

	_, err := findUVImpl("", cacheDir)
	if err == nil {
		t.Fatal("expected error when installUVFn fails")
	}
}

func TestFindUVImplRenameFails(t *testing.T) {
	tmp := t.TempDir()
	cacheDir := filepath.Join(tmp, "cache")
	os.MkdirAll(cacheDir, 0o755) //nolint:errcheck
	// Make cacheDir read-only so rename into it fails.
	os.Chmod(cacheDir, 0o555)       //nolint:errcheck
	defer os.Chmod(cacheDir, 0o755) //nolint:errcheck

	origInstall := installUVFn
	installUVFn = func(dir string) error {
		return os.WriteFile(filepath.Join(dir, "uv"), []byte("#!/bin/sh\n"), 0o755)
	}
	defer func() { installUVFn = origInstall }()

	_, err := findUVImpl("", cacheDir)
	if err == nil {
		t.Fatal("expected error when rename fails")
	}
}
