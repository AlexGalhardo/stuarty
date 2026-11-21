import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import { SessionProvider } from "@/lib/session";
import type { SocialAccount, SocialNetwork } from "../../editor/api/types";
import { SetupGuide } from "./SetupGuide";
import { AccountPicker, AccountRow } from "./SocialSection";

vi.mock("@inertiajs/react", () => ({ Head: () => null, Link: () => null }));

const PAYLOAD = "<img src=x onerror=window.__pwned=1>";

const messages = {
	"ui.session.person_only.local": "Only a person in the app can do this.",
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
	"ui.social.needs_account": "Signed in. Choose the account.",
	"ui.social.pick_title": "Choose the account",
	"ui.social.pick_hint": "Your sign-in reached these accounts.",
	"ui.social.pick_use": "Use this account",
	"ui.social.instagram.step.project": "Create a Business app.",
	"ui.social.instagram.step.api": "Add Facebook Login.",
	"ui.social.instagram.step.consent": "Use a professional account.",
	"ui.social.instagram.step.client": "Copy the App ID and the App Secret.",
	"ui.social.instagram.step.redirect": "Add exactly: {redirect}",
	"ui.social.instagram.step.paste": "Paste them. Permissions: {scopes}.",
	"ui.social.instagram.research":
		"What Meta said on {read}: the file needs no public address.",
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

const instagram: SocialNetwork = {
	...network,
	network: "instagram",
	name: "Instagram",
	publishes: ["video"],
	endpoint: "https://graph.facebook.com",
	hosts: ["graph.facebook.com", "rupload.facebook.com"],
	scopes: ["pages_show_list", "instagram_content_publish"],
	guide:
		"https://developers.facebook.com/docs/instagram-platform/content-publishing",
	ai_disclosure: true,
	limits: {
		...network.limits,
		formats: ["mp4", "mov"],
		visibilities: ["public"],
		needs_kids_declaration: false,
		thumbnail_formats: [],
	},
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

describe("the price of a post", () => {
	const priced = account({
		network: "x",
		price: {
			currency: "USD",
			post_micros: 15_000,
			link_micros: 200_000,
			source: "https://docs.x.com/x-api/getting-started/pricing",
			read: "2026-11-20",
		},
	});

	test("an account of a network that charges shows the price with its date and a form to correct it", () => {
		const html = wrap(
			<ul>
				<AccountRow
					account={priced}
					networkName="X"
					onSignInAgain={() => undefined}
					onPublish={() => undefined}
					onPrice={() => undefined}
				/>
			</ul>,
		);
		expect(html).toContain("ui.social.price_now");
		expect(html).toContain('value="0.015"');
		expect(html).toContain('value="0.2"');
		expect(html).toContain("data-price-editor");
	});

	test("an account of a network that charges nothing has no price form", () => {
		const html = wrap(
			<ul>
				<AccountRow
					account={account()}
					networkName="YouTube"
					onSignInAgain={() => undefined}
					onPublish={() => undefined}
					onPrice={() => undefined}
				/>
			</ul>,
		);
		expect(html).not.toContain("data-price-editor");
	});
});

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

	test("a sign-in that reached several accounts asks which one, and cannot publish until chosen", () => {
		const html = wrap(
			<ul>
				<AccountRow
					account={account({
						network: "instagram",
						state: "needs_account",
						account_title: undefined,
						choices: [
							{ id: "IG1", title: "@first (First Page)" },
							{ id: "IG2", title: PAYLOAD },
						],
					})}
					networkName="Instagram"
					onSignInAgain={() => undefined}
					onPublish={() => undefined}
					onSelect={() => undefined}
				/>
			</ul>,
		);
		expect(html).toContain("Signed in. Choose the account.");
		expect(html).toContain("Choose the account");
		expect(html).toContain("@first (First Page)");
		expect((html.match(/type="radio"/g) ?? []).length).toBe(2);
		// A title the network wrote is text, never markup.
		expect(html).not.toContain(PAYLOAD);
		expect(html).toContain("&lt;img");
		// Nothing is chosen for the person: the confirm button and the publish button wait.
		expect(html).toMatch(/<button[^>]*disabled[^>]*>\s*Use this account/);
		expect(html).toMatch(/<button[^>]*disabled[^>]*>\s*Publish now/);
		expect(html).not.toMatch(/type="radio"[^>]*checked/);
	});

	test("an account that is ready shows no picker", () => {
		const html = wrap(
			<ul>
				<AccountRow
					account={account({ choices: [{ id: "x", title: "x" }] })}
					networkName="Instagram"
					onSignInAgain={() => undefined}
					onPublish={() => undefined}
					onSelect={() => undefined}
				/>
			</ul>,
		);
		expect(html).not.toContain("data-account-picker");
	});

	test("the picker names its radios by the connection, so two connections do not share a choice", () => {
		const html = wrap(
			<AccountPicker
				connectionId="con_9"
				choices={[{ id: "P1", title: "Page" }]}
				busy={false}
				onSelect={() => undefined}
			/>,
		);
		expect(html).toContain('name="social-account-con_9"');
		expect(html).toContain('data-account-picker="con_9"');
	});

	test("a session that is not a person's sees the choice disabled, with the reason (VV-331)", () => {
		const html = wrap(
			<AccountPicker
				connectionId="con_9"
				choices={[{ id: "P1", title: "Page" }]}
				busy={false}
				onSelect={() => undefined}
			/>,
		);
		expect(html).toMatch(/<button[^>]*disabled[^>]*>\s*Use this account/);
		expect(html).toContain('id="pick-person-only-con_9"');
		expect(html).toContain("Only a person");
	});

	test("a person's session sees it enabled and no reason (VV-331)", () => {
		const html = wrap(
			<SessionProvider session={{ privileged: true, mode: "local" }}>
				<AccountPicker
					connectionId="con_9"
					choices={[{ id: "P1", title: "Page" }]}
					busy={false}
					onSelect={() => undefined}
				/>
			</SessionProvider>,
		);
		expect(html).not.toContain("person-only");
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

	test("a Meta network has its own steps, the date read and the page of Meta", () => {
		const html = wrap(
			<SetupGuide
				network={instagram}
				redirectPath="/social/callback"
				origin="http://127.0.0.1:7878"
				onClose={() => undefined}
				navigate={() => undefined}
			/>,
		);
		expect(html).toContain("Create a Business app.");
		expect(html).toContain("Use a professional account.");
		expect(html).toContain(
			"Add exactly: http://127.0.0.1:7878/social/callback",
		);
		expect(html).toContain("pages_show_list, instagram_content_publish");
		expect(html).toContain("What Meta said on 2026-11-19");
		expect(html).toContain(instagram.guide);
		expect(html).not.toContain("Google");
	});
});
