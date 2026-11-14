package workspace

import (
	"errors"
	"os"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func loosen(t *testing.T, path string) {
	t.Helper()
	user, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	everyone, err := windows.CreateWellKnownSid(windows.WinWorldSid)
	if err != nil {
		t.Fatal(err)
	}
	if err := setProtectedDACL(path, []*windows.SID{user, everyone}); err != nil {
		t.Fatal(err)
	}
}

// aclSIDs returns the SIDs of the allow entries of a file and whether its
// DACL is protected.
func aclSIDs(t *testing.T, path string) (sids []*windows.SID, protected bool) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		t.Fatalf("DACL: %v", err)
	}
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			t.Fatal(err)
		}
		sids = append(sids, (*windows.SID)(unsafe.Pointer(&ace.SidStart)))
	}
	return sids, control&windows.SE_DACL_PROTECTED != 0
}

func TestSecurity_T03_TokenFileACLOwnerOnly(t *testing.T) {
	t.Parallel()
	d := newDir(t)
	if _, err := d.EnsureToken(); err != nil {
		t.Fatal(err)
	}

	sids, protected := aclSIDs(t, d.Path("token"))
	if !protected {
		t.Error("the DACL must be protected: no inherited entries")
	}
	user, err := currentUserSID()
	if err != nil {
		t.Fatal(err)
	}
	if len(sids) != 1 || !sids[0].Equals(user) {
		t.Errorf("DACL entries = %v, want only the current user %s", sids, user)
	}
	if err := checkTokenFile(d.Path("token")); err != nil {
		t.Errorf("checkTokenFile: %v", err)
	}
}

func TestTokenFileWithInheritedACLRefused(t *testing.T) {
	t.Parallel()
	d := newDir(t)
	// A plain file inherits the folder ACL, which usually has more than the user.
	if err := os.WriteFile(d.Path("token"), []byte("vvl_"+string(make([]byte, 64))+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := d.LoadToken(); !errors.Is(err, ErrTokenPermissions) {
		t.Fatalf("LoadToken = %v, want ErrTokenPermissions", err)
	}
}
