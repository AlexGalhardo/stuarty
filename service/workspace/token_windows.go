package workspace

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The DACL is read and written through x/sys/windows, not through icacls as
// SDD-007 section 3.2 first sketched: the author approved the direct
// dependency, and it needs no child process and no PATH lookup.

func currentUserSID() (*windows.SID, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return u.User.Sid, nil
}

func allowedSIDs() ([]*windows.SID, error) {
	user, err := currentUserSID()
	if err != nil {
		return nil, err
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return nil, err
	}
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return nil, err
	}
	return []*windows.SID{user, system, admins}, nil
}

// restrictTokenFile replaces the DACL with a protected one that grants full
// control to the current user only.
func restrictTokenFile(path string) error {
	user, err := currentUserSID()
	if err != nil {
		return fmt.Errorf("current user: %w", err)
	}
	return setProtectedDACL(path, []*windows.SID{user})
}

func setProtectedDACL(path string, sids []*windows.SID) error {
	entries := make([]windows.EXPLICIT_ACCESS, 0, len(sids))
	for _, sid := range sids {
		entries = append(entries, windows.EXPLICIT_ACCESS{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       windows.NO_INHERITANCE,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_UNKNOWN,
				TrusteeValue: windows.TrusteeValueFromSID(sid),
			},
		})
	}
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return fmt.Errorf("build ACL: %w", err)
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil)
}

// checkTokenFile requires a protected DACL (no inherited entries) whose
// entries are allow entries for the current user, SYSTEM or Administrators.
func checkTokenFile(path string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrTokenPermissions, path)
	}

	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("workspace: read ACL of %s: %w", path, err)
	}
	control, _, err := sd.Control()
	if err != nil {
		return fmt.Errorf("workspace: read ACL of %s: %w", path, err)
	}
	fix := fmt.Sprintf("run: icacls %q /inheritance:r /grant:r \"%%USERNAME%%:F\"", path)
	if control&windows.SE_DACL_PROTECTED == 0 {
		return fmt.Errorf("%w: the ACL inherits entries from the folder; %s", ErrTokenPermissions, fix)
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		return fmt.Errorf("%w: the file has no DACL; %s", ErrTokenPermissions, fix)
	}

	allowed, err := allowedSIDs()
	if err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	for i := range uint32(acl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			return fmt.Errorf("workspace: read ACE: %w", err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			return fmt.Errorf("%w: unexpected entry type %d; %s", ErrTokenPermissions, ace.Header.AceType, fix)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sidIn(allowed, sid) {
			return fmt.Errorf("%w: %s has access; %s", ErrTokenPermissions, sid.String(), fix)
		}
	}
	return nil
}

func sidIn(set []*windows.SID, sid *windows.SID) bool {
	for _, s := range set {
		if s.Equals(sid) {
			return true
		}
	}
	return false
}
