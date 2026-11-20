// The fake social network of the e2e run. The server of the suite knows one address for it
// (STUARTY_TEST_SOCIAL_URL, playwright.config.ts), and three specs play a network there: youtube-connect,
// meta-connect and publish-calendar. Their files run in parallel workers, so a spec asks for the port until
// the spec that holds it lets it go; the calendar holds it for as long as a post takes to come due.
import type { Server } from "node:http";
import { expect, test } from "@playwright/test";

export const SOCIAL_PORT = Number(process.env.STUARTY_E2E_PORT ?? 3101) + 3;

// The longest a spec waits for the port: every other test that holds it, one after the other.
const PORT_WAIT_MS = 300_000;

// listenWhenFree binds the server to the port of the fake network once no other spec holds it. The wait is
// added to the time of the test, so a test is never failed by the length of another one.
export async function listenWhenFree(server: Server): Promise<void> {
	test.setTimeout(test.info().timeout + PORT_WAIT_MS);
	const tryListen = (): Promise<boolean> =>
		new Promise((resolve, reject) => {
			const failed = (error: NodeJS.ErrnoException): void => {
				if (error.code === "EADDRINUSE") resolve(false);
				else reject(error);
			};
			server.once("error", failed);
			server.listen(SOCIAL_PORT, "127.0.0.1", () => {
				server.off("error", failed);
				resolve(true);
			});
		});
	await expect
		.poll(tryListen, { timeout: PORT_WAIT_MS, intervals: [500] })
		.toBe(true);
}
