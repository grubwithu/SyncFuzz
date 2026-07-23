package environment

import "testing"

func TestSandboxUserIDsForRunner(t *testing.T) {
	uid, gid := sandboxUserIDsForRunner(1000, 1001, "0", "0")
	if uid != 1000 || gid != 1001 {
		t.Fatalf("expected unprivileged runner identity, got %d:%d", uid, gid)
	}
	uid, gid = sandboxUserIDsForRunner(0, 0, "1000", "1001")
	if uid != 1000 || gid != 1001 {
		t.Fatalf("expected sudo caller identity, got %d:%d", uid, gid)
	}
	uid, gid = sandboxUserIDsForRunner(0, 0, "invalid", "1001")
	if uid != 0 || gid != 0 {
		t.Fatalf("expected root fallback for invalid sudo identity, got %d:%d", uid, gid)
	}
}
