// Package crashharness holds the kill-point checks of SDD-008 section 13.3 that need no server: it starts a
// child process built with the crashtest tag, lets a named crash point end it with exit code 137 (as a SIGKILL
// would), and reopens what the child left to assert that nothing is corrupt and nothing acknowledged is lost.
//
// The points of the commit path, the bundle export and import, the migration and the emptying of the trash are
// killed by the tests of web/app/servicetest (build tag crashtest); this package kills the vault and checks that
// every crash point of the code is covered by one of those tests (TestEveryCrashPointHasARecoveryTest), so a new
// point cannot be added without a test that kills it. `bash scripts/crash.sh` runs them all.
package crashharness
