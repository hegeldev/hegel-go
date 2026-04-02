package hegel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- serverLogExcerpt ---

func TestServerLogExcerptEmptyPath(t *testing.T) {
	t.Parallel()
	result := serverLogExcerpt("")
	if result != "" {
		t.Errorf("expected empty for empty path, got %q", result)
	}
}

func TestServerLogExcerptMissingFile(t *testing.T) {
	t.Parallel()
	result := serverLogExcerpt("/nonexistent/file.log")
	if result != "" {
		t.Errorf("expected empty for missing file, got %q", result)
	}
}

func TestServerLogExcerptEmptyFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "server.log")
	os.WriteFile(logFile, []byte(""), 0o644) //nolint:errcheck

	result := serverLogExcerpt(logFile)
	if result != "" {
		t.Errorf("expected empty for empty file, got %q", result)
	}
}

func TestServerLogExcerptNonEmptyFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "server.log")
	os.WriteFile(logFile, []byte("Error: test crash\n"), 0o644) //nolint:errcheck

	result := serverLogExcerpt(logFile)
	if result == "" {
		t.Error("expected non-empty excerpt")
	}
	if !strings.Contains(result, "Error: test crash") {
		t.Errorf("excerpt should contain log content, got %q", result)
	}
}

// --- serverCrashMessage ---

func TestServerCrashMessageIncludesLogExcerpt(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "server.log")
	os.WriteFile(logFile, []byte("Error: test crash\n"), 0o644) //nolint:errcheck
	f, _ := os.Open(logFile)
	defer f.Close()

	s := newHegelSession()
	s.logFile = f

	msg := s.serverCrashMessage()
	if !strings.Contains(msg, "Error: test crash") {
		t.Errorf("crash message should include log content, got: %s", msg)
	}
	if !strings.Contains(msg, "server process exited unexpectedly") {
		t.Errorf("crash message should include base message, got: %s", msg)
	}
}

func TestServerCrashMessageEmptyLog(t *testing.T) {
	tmp := t.TempDir()
	logFile := filepath.Join(tmp, "server.log")
	os.WriteFile(logFile, []byte(""), 0o644) //nolint:errcheck
	f, _ := os.Open(logFile)
	defer f.Close()

	s := newHegelSession()
	s.logFile = f

	msg := s.serverCrashMessage()
	if !strings.Contains(msg, "No entries found") {
		t.Errorf("expected 'No entries found' for empty log, got: %s", msg)
	}
}

// --- serverHasExited ---

func TestServerHasExitedNilChannel(t *testing.T) {
	t.Parallel()
	s := newHegelSession()
	if s.serverHasExited() {
		t.Error("expected false for nil processExited")
	}
}

func TestServerHasExitedNotYet(t *testing.T) {
	t.Parallel()
	s := newHegelSession()
	s.processExited = make(chan struct{})
	if s.serverHasExited() {
		t.Error("expected false for open processExited channel")
	}
}

func TestServerHasExitedTrue(t *testing.T) {
	t.Parallel()
	s := newHegelSession()
	ch := make(chan struct{})
	close(ch)
	s.processExited = ch
	if !s.serverHasExited() {
		t.Error("expected true for closed processExited channel")
	}
}

// --- Fake server crash tests ---

// fakeServerWithLog is a Python script that completes the handshake,
// writes a diagnostic message to stderr, then exits before responding
// to run_test. This simulates a server crash with log output.
const fakeServerWithLog = `#!/usr/bin/env python3
import sys, struct, binascii

MAGIC = 0x4845474C
REPLY_BIT = 0x80000000
TERM = b'\x0a'

def read_exact(n):
    data = b''
    while len(data) < n:
        chunk = sys.stdin.buffer.read(n - len(data))
        if not chunk: return None
        data += chunk
    return data

def read_packet():
    hdr = read_exact(20)
    if hdr is None: return None
    channel = struct.unpack('>I', hdr[8:12])[0]
    mid_raw = struct.unpack('>I', hdr[12:16])[0]
    length = struct.unpack('>I', hdr[16:20])[0]
    message_id = mid_raw & ~REPLY_BIT
    payload = read_exact(length)
    if payload is None: return None
    read_exact(1)
    return channel, message_id, payload

def write_packet(channel, message_id, payload):
    if isinstance(payload, str): payload = payload.encode()
    mid = message_id | REPLY_BIT
    hdr = struct.pack('>IIIII', MAGIC, 0, channel, mid, len(payload))
    csum = binascii.crc32(hdr + payload) & 0xFFFFFFFF
    hdr = hdr[:4] + struct.pack('>I', csum) + hdr[8:]
    sys.stdout.buffer.write(hdr + payload + TERM)
    sys.stdout.buffer.flush()

sys.stderr.write("FakeServerError: intentional crash for testing\n")
sys.stderr.flush()
pkt = read_packet()
if pkt is None: sys.exit(1)
channel, message_id, _ = pkt
write_packet(channel, message_id, b"Hegel/0.6")
sys.exit(1)
`

// fakeServerNoLog is the same but writes nothing to stderr.
const fakeServerNoLog = `#!/usr/bin/env python3
import sys, struct, binascii

MAGIC = 0x4845474C
REPLY_BIT = 0x80000000
TERM = b'\x0a'

def read_exact(n):
    data = b''
    while len(data) < n:
        chunk = sys.stdin.buffer.read(n - len(data))
        if not chunk: return None
        data += chunk
    return data

def read_packet():
    hdr = read_exact(20)
    if hdr is None: return None
    channel = struct.unpack('>I', hdr[8:12])[0]
    mid_raw = struct.unpack('>I', hdr[12:16])[0]
    length = struct.unpack('>I', hdr[16:20])[0]
    message_id = mid_raw & ~REPLY_BIT
    payload = read_exact(length)
    if payload is None: return None
    read_exact(1)
    return channel, message_id, payload

def write_packet(channel, message_id, payload):
    if isinstance(payload, str): payload = payload.encode()
    mid = message_id | REPLY_BIT
    hdr = struct.pack('>IIIII', MAGIC, 0, channel, mid, len(payload))
    csum = binascii.crc32(hdr + payload) & 0xFFFFFFFF
    hdr = hdr[:4] + struct.pack('>I', csum) + hdr[8:]
    sys.stdout.buffer.write(hdr + payload + TERM)
    sys.stdout.buffer.flush()

pkt = read_packet()
if pkt is None: sys.exit(1)
channel, message_id, _ = pkt
write_packet(channel, message_id, b"Hegel/0.6")
sys.exit(1)
`

func makeFakeServer(t *testing.T, script string) string {
	t.Helper()
	tmp := t.TempDir()
	scriptPath := filepath.Join(tmp, "fake_server.py")
	os.WriteFile(scriptPath, []byte(script), 0o644) //nolint:errcheck

	wrapperPath := filepath.Join(tmp, "server")
	wrapper := "#!/bin/sh\npython3 " + scriptPath + " \"$@\"\n"
	os.WriteFile(wrapperPath, []byte(wrapper), 0o755) //nolint:errcheck

	return wrapperPath
}

func TestServerCrashIncludesLogContent(t *testing.T) {
	serverPath := makeFakeServer(t, fakeServerWithLog)

	s := newHegelSession()
	s.hegelCmd = serverPath
	defer s.cleanup()
	if err := s.start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	err := s.runTest(func(tc *TestCase) {
		Draw[bool](tc, Booleans())
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err == nil {
		t.Fatal("expected error from fake crash server")
	}
	mustContainStr(t, err.Error(), "FakeServerError: intentional crash for testing")
}

func TestServerCrashEmptyLog(t *testing.T) {
	serverPath := makeFakeServer(t, fakeServerNoLog)

	s := newHegelSession()
	s.hegelCmd = serverPath
	defer s.cleanup()
	if err := s.start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	err := s.runTest(func(tc *TestCase) {
		Draw[bool](tc, Booleans())
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err == nil {
		t.Fatal("expected error from fake crash server")
	}
	mustContainStr(t, err.Error(), "server process exited unexpectedly")
}

// --- testKillServer with no server ---

func TestKillServerNoProcess(t *testing.T) {
	old := globalSession
	defer func() { globalSession = old }()
	globalSession = newHegelSession()
	// Should not panic when no server is running.
	testKillServer()
}

// --- Server restart after kill ---

func TestServerRestartsAfterKill(t *testing.T) {
	hegelBinPath(t)

	// First run — starts the server and completes successfully.
	err := Run(func(s *TestCase) {
		Draw[bool](s, Booleans())
	}, WithTestCases(1))
	if err != nil {
		t.Fatalf("first run: %v", err)
	}

	// Kill the server and wait for the connection to detect it has exited.
	testKillServer()

	// Second run — should detect the dead session, restart the server, and succeed.
	err = Run(func(s *TestCase) {
		Draw[bool](s, Booleans())
	}, WithTestCases(1))
	if err != nil {
		t.Fatalf("second run after restart: %v", err)
	}
}
