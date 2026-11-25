import { tmpdir } from "node:os";
import { join } from "node:path";
import type { AccountTarget } from "./accountSetup";

// The local servers of the run and the files that keep their signed-in sessions. They follow the same
// variables as playwright.config.ts, which reads this file too, so the two never disagree.
const port = Number(process.env.STUARTY_E2E_PORT ?? 3101);
export const home =
	process.env.STUARTY_E2E_HOME ?? join(tmpdir(), "stuarty-e2e-home");
export const failHome = `${home}-failing-encoder`;
// The server of the specs that sign in with the password (a sign-in ends every session of its server).
export const personHome = `${home}-person`;
export const personPort = port + 9;

export const mainStatePath = `${home}.state.json`;
export const failStatePath = `${failHome}.state.json`;
export const personStatePath = `${personHome}.state.json`;

export function accountTargets(): AccountTarget[] {
	return [
		{
			baseURL: `http://127.0.0.1:${port}`,
			home,
			statePath: mainStatePath,
		},
		{
			baseURL: `http://127.0.0.1:${port + 1}`,
			home: failHome,
			statePath: failStatePath,
		},
		{
			baseURL: `http://127.0.0.1:${personPort}`,
			home: personHome,
			statePath: personStatePath,
		},
	];
}
