// Specs that run in no project of the browser matrix (SDD-012 section 20.1):
// the load-sensitive budgets and the failing-encoder and detection servers.
export const NOT_IN_MATRIX =
	/(perf|video-layers|export-failure|desktop-detect|desktop-detect-webkit)\.spec\.ts/;

// A test whose title carries this tag runs in Chromium only. Each use has the
// reason in a comment next to it: an engine limit, never a defect to hide.
export const CHROMIUM_ONLY = "@chromium-only";
