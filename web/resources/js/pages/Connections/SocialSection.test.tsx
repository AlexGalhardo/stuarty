import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import type { SocialAccount, SocialNetwork } from "../../editor/api/types";
import { SetupGuide } from "./SetupGuide";
import { AccountRow } from "./SocialSection";

vi.mock("@inertiajs/react", () => ({ Head: () => null, Link: () => null }));

const PAYLOAD = "<img src=x onerror=window.__pwned=1>";

const messages = {
	"ui.social.signed_in_as": "Signed in as",
	"ui.social.needs_sign_in": "Not signed in yet",
	"ui.social.sign_in_again": "Sign in again",
	"ui.social.publish_now": "Publish now",
	"ui.social.guide_title": "Connect {name} with your own developer project",
	"ui.social.guide_intro": "You create the app in your own account.",
	"ui.social.redirect_label": "Redirect address to paste",
	"ui.social.official_page": "Read the official page",
	"ui.social.client_id": "Client id",
	"ui.social.client_secret": "Client secret",
	"ui.social.account_label": "Name",
	"ui.social.start_sign_in": "Save and sign in",
	"ui.social.close": "Close",
	"ui.connections.secret_hint": "Stored encrypted on this machine.",
	"ui.social.youtube.step.project": "Create a project.",
	"ui.social.youtube.step.api": "Enable the API.",
	"ui.social.youtube.step.consent": "Configure the consent screen.",
	"ui.social.youtube.step.client": "Create a Web application client.",
	"ui.social.youtube.step.redirect": "Paste exactly: {redirect}",
	"ui.social.youtube.step.paste":
		"Copy the id and the secret. Permissions: {scopes}.",
	"ui.social.youtube.research":
		"What Google said on {read}: unverified projects are no longer forced to private.",
};

const network: SocialNetwork = {
	network: "youtube",
	name: "YouTube",
	publishes: ["video", "thumbnail"],
	limits: {
		title_max: 100,
		description_max_bytes: 5000,
		tags_max_chars: 500,
		file_max_bytes: 1,
		formats: ["mp4"],
		source: "https://developers.google.com/youtube/v3/docs/videos",
		read: "2026-11-19",
	},
	endpoint: "https://www.googleapis.com",
	hosts: ["www.googleapis.com"],
	scopes: ["youtube.upload", "youtube.readonly"],
	guide:
		"https://developers.google.com/youtube/v3/guides/auth/server-side-web-apps",
	guide_read: "2026-11-19",
};

const account = (extra: Partial<SocialAccount> = {}): SocialAccount => ({
	connection_id: "con_1",
	network: "youtube",
	label: "My channel",
	account_title: "Studio",
	state: "ready",
	...extra,
});

function wrap(node: React.ReactNode): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			{node}
		</I18nProvider>,
	);
}

describe("the accounts of the social networks", () => {
	test("a label and a channel name from the network are text, never markup", () => {
		const html = wrap(
			<ul>
				<AccountRow
					account={account({ label: PAYLOAD, account_title: PAYLOAD })}
					networkName="YouTube"
					onSignInAgain={() => undefined}
					onPublish={() => undefined}
				/>
			</ul>,
		);
		expect(html).not.toContain(PAYLOAD);
		expect(html).toContain("&lt;img");
		expect(html).toContain("Signed in as");
	});

	test("an account that is not signed in cannot publish and can sign in again", () => {
		const html = wrap(
			<ul>
				<AccountRow
					account={account({
						state: "needs_sign_in",
						account_title: undefined,
					})}
					networkName="YouTube"
					onSignInAgain={() => undefined}
					onPublish={() => undefined}
				/>
			</ul>,
		);
		expect(html).toContain("Not signed in yet");
		expect(html).toMatch(/<button[^>]*disabled[^>]*>\s*Publish now/);
		expect(html).toContain("Sign in again");
	});
});

describe("the setup guide of a network", () => {
	function guide(connectionId?: string): string {
		return wrap(
			<SetupGuide
				network={network}
				redirectPath="/social/callback"
				connectionId={connectionId}
				origin="http://127.0.0.1:7878"
				onClose={() => undefined}
				navigate={() => undefined}
			/>,
		);
	}

	test("shows the exact redirect address, the steps, the date read and the official page", () => {
		const html = guide();
		expect(html).toContain("http://127.0.0.1:7878/social/callback");
		expect(html).toContain(
			"Paste exactly: http://127.0.0.1:7878/social/callback",
		);
		expect(html).toContain("youtube.upload, youtube.readonly");
		expect(html).toContain("What Google said on 2026-11-19");
		expect(html).toContain(network.guide);
		expect(html).toContain('rel="noreferrer noopener"');
	});

	test("the client secret is asked in a password field", () => {
		expect(guide()).toMatch(
			/id="social-client-secret"[^>]*type="password"|type="password"[^>]*id="social-client-secret"/,
		);
	});

	test("signing in again asks for nothing", () => {
		const html = guide("con_1");
		expect(html).not.toContain("social-client-id");
		expect(html).toContain("Sign in again");
	});
});
