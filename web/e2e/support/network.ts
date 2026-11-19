// The network-mode server of the e2e run (playwright.config.ts) and the
// administrator it starts with. Two ports above the local server, whose own
// failing-encoder twin takes the one in between.
export const NETWORK_PORT = Number(process.env.STUARTY_E2E_PORT ?? 3101) + 2;
export const NETWORK_URL = `http://127.0.0.1:${NETWORK_PORT}`;
export const NETWORK_ADMIN = {
	email: "admin@example.test",
	password: "e2e-network-password-1",
};
