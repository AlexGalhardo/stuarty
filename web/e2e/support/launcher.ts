// The launcher secret of the main server of the suite: a throwaway value, not a credential. A spec
// that needs a person's session (the approval queue, SDD-015) sends it as the Stuarty-Launcher header.
export const LAUNCHER_SECRET = "e2e-launcher-secret-not-a-credential";
