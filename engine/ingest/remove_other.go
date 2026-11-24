//go:build !windows

package ingest

// transientRemoveError is always false here: a file another process holds open can be removed, so an access denied
// or a folder that is not empty is a real answer (a permission, a writer still in the folder) and is not waited for.
func transientRemoveError(error) bool {
	return false
}
