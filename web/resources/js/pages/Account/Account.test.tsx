import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import FieldErrors from "@/components/FieldErrors";
import PasswordField from "@/components/PasswordField";
import { I18nProvider } from "@/lib/i18n";
import Create, { RecoveryKey } from "./Create";
import Recover from "./Recover";
import SignIn from "./SignIn";

// Head needs the Inertia app; the page content does not. The sign-in reads the account state the server shares.
const shared = vi.hoisted(() => ({
	account: { state: "locked", username: "ana" } as {
		state: string;
		username: string;
	},
}));
vi.mock("@inertiajs/react", () => ({
	Head: () => null,
	usePage: () => ({ props: { account: shared.account } }),
}));

const messages = {
	"ui.account.show": "Show password",
	"ui.account.hide": "Hide password",
	"ui.account.username": "Username",
	"ui.account.email": "E-mail",
	"ui.account.password": "Password",
	"ui.account.confirmation": "Confirm the password",
	"ui.account.identity": "Username or e-mail",
	"ui.account.create.title": "Create your account",
	"ui.account.create.not_encrypted":
		"Not encrypted yet: media files, names, timeline text.",
	"ui.account.signin.title": "Sign in to Stuarty",
	"ui.account.recover.title": "Recover with the recovery key",
	"ui.account.recover.key": "Recovery key",
	"ui.account.reason.too_short": "It is too short.",
	"ui.account.reason.no_digit": "Add a digit.",
};

function render(node: React.ReactNode): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			{node}
		</I18nProvider>,
	);
}

describe("what the account does not encrypt", () => {
	test("the creation page says it before the person trusts it", () => {
		expect(render(<Create />)).toContain(
			"Not encrypted yet: media files, names, timeline text.",
		);
	});
});

describe("the password field", () => {
	const html = render(
		<PasswordField
			id="p"
			label="Password"
			value=""
			onChange={() => undefined}
			autoComplete="new-password"
		/>,
	);

	test("hides what is typed and has an eye to show it", () => {
		expect(html).toMatch(/<input[^>]*type="password"[^>]*id="p"/);
		const eye = html.match(/<button[^>]*data-testid="p-eye"[^>]*>/)?.[0] ?? "";
		expect(eye).toContain('aria-label="Show password"');
		expect(eye).toContain('aria-pressed="false"');
		expect(eye).toContain('type="button"');
	});

	test("lets a browser and a password manager fill it", () => {
		expect(html).toContain('autoComplete="new-password"');
		expect(html).toMatch(/<label[^>]*for="p"/);
	});

	test("writes the reasons the server gave under the field and points the field at them", () => {
		const bad = render(
			<PasswordField
				id="p"
				label="Password"
				value="x"
				onChange={() => undefined}
				autoComplete="new-password"
				reasons={["too_short", "no_digit"]}
			/>,
		);
		expect(bad).toContain("It is too short.");
		expect(bad).toContain("Add a digit.");
		expect(bad).toContain('aria-invalid="true"');
		expect(bad).toContain('aria-describedby="p-errors"');
		expect(bad).toContain('id="p-errors"');
	});
});

describe("the field errors", () => {
	test("write nothing when there is no reason", () => {
		expect(render(<FieldErrors id="e" reasons={[]} />)).toBe("");
	});
});

describe("the create-account page", () => {
	const html = render(<Create />);

	test("asks for the four fields, each with a label", () => {
		for (const id of [
			"account-username",
			"account-email",
			"account-password",
			"account-confirmation",
		]) {
			expect(html).toMatch(new RegExp(`<label[^>]*for="${id}"`));
			expect(html).toMatch(new RegExp(`<input[^>]*id="${id}"`));
		}
	});

	test("puts the eye on both password fields", () => {
		expect(html).toContain('data-testid="account-password-eye"');
		expect(html).toContain('data-testid="account-confirmation-eye"');
		expect(html.match(/type="password"/g)?.length).toBe(2);
	});

	test("names the purpose of the username and the e-mail for the browser", () => {
		expect(html).toContain('autoComplete="username"');
		expect(html).toContain('autoComplete="email"');
	});

	test("does not show the recovery key before the account exists", () => {
		expect(html).not.toContain("recovery-key");
	});
});

describe("the sign-in and recovery pages", () => {
	test("the sign-in asks for the username or the e-mail and a password with an eye", () => {
		const html = render(<SignIn />);
		expect(html).toMatch(/<input[^>]*id="signin-identity"/);
		expect(html).toContain('autoComplete="current-password"');
		expect(html).toContain('data-testid="signin-password-eye"');
		expect(html).toContain('href="/account/recover"');
	});

	test("the sign-in says the studio is locked only when the server says so", () => {
		const withIntro = (node: React.ReactNode): string =>
			renderToString(
				<I18nProvider
					messages={{
						...messages,
						"ui.account.signin.intro": "Locked until you sign in.",
						"ui.account.signin.intro_open": "Sign in as yourself.",
					}}
					locale="en"
					locales={["en"]}
				>
					{node}
				</I18nProvider>,
			);
		const locked = withIntro(<SignIn />);
		expect(locked).toContain("Locked until you sign in.");
		shared.account = { state: "unlocked", username: "ana" };
		const open = withIntro(<SignIn />);
		shared.account = { state: "locked", username: "ana" };
		expect(open).toContain("Sign in as yourself.");
		expect(open).not.toContain("Locked until");
	});

	test("the recovery asks for the key and a new password twice, with an eye on each", () => {
		const html = render(<Recover />);
		expect(html).toMatch(/<input[^>]*id="recover-key"/);
		expect(html).toContain('data-testid="recover-password-eye"');
		expect(html).toContain('data-testid="recover-confirmation-eye"');
	});
});

describe("the recovery key step", () => {
	const withMessages = {
		...messages,
		"ui.account.recovery.title": "Save your recovery key",
		"ui.account.recovery.formerFolder": "A clear copy is still in {path}.",
	};

	function renderStep(formerFolder: string | null): string {
		return renderToString(
			<I18nProvider messages={withMessages} locale="en" locales={["en"]}>
				<RecoveryKey value="ABCD-EFGH" formerFolder={formerFolder} />
			</I18nProvider>,
		);
	}

	test("tells the person where the clear copy of an adoption by copy is (VV-379)", () => {
		const html = renderStep("/home/x/.vukem-video");
		expect(html).toContain('data-testid="former-folder"');
		expect(html).toContain("A clear copy is still in /home/x/.vukem-video.");
	});

	test("says nothing about a folder when there is none", () => {
		expect(renderStep(null)).not.toContain("former-folder");
	});
});

describe("the first-run steps and the confirmation of the key", () => {
	const stepMessages = {
		...messages,
		"ui.account.steps.create": "Create account",
		"ui.account.steps.key": "Keep the key",
		"ui.account.steps.signin": "Sign in",
		"ui.account.steps.done": "Done",
		"ui.account.recovery.continue": "Continue to sign in",
		"ui.account.recovery.saved": "I saved my recovery key",
	};

	function inSteps(node: React.ReactNode): string {
		return renderToString(
			<I18nProvider messages={stepMessages} locale="en" locales={["en"]}>
				{node}
			</I18nProvider>,
		);
	}

	test("the creation is the current step and the other two wait", () => {
		const html = inSteps(<Create />);
		expect(html.match(/data-state="current"/g)).toHaveLength(1);
		expect(html.match(/data-state="todo"/g)).toHaveLength(2);
		expect(html).toContain('aria-current="step"');
	});

	test("the key step has the first step done with its word, and the way on is disabled until the box is ticked", () => {
		const html = inSteps(<RecoveryKey value="ABCD-EFGH" formerFolder={null} />);
		expect(html).toContain('data-state="done"');
		expect(html).toContain("Done");
		expect(html).toContain('data-testid="recovery-key"');
		expect(html).toMatch(/<button[^>]*disabled[^>]*>.*Continue to sign in/s);
		expect(html).toMatch(
			/<input[^>]*id="recovery-saved"[^>]*type="checkbox"|<input[^>]*type="checkbox"[^>]*id="recovery-saved"/,
		);
	});
});
