import { signInEveryServer } from "./accountSetup";
import { accountTargets } from "./accountTargets";

// The global setup of the suite (playwright.config.ts): the account of every local server, and its session.
export default async function globalSetup(): Promise<void> {
	await signInEveryServer(accountTargets());
}
