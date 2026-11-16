import { createRoot } from "react-dom/client";

import "../css/app.css";
import { createClient } from "./editor/api/client";
import { HttpTransport } from "./editor/api/http";
import {
	type EditorBootstrap,
	ownActor,
	parseBootstrap,
} from "./editor/boot/bootstrap";
import { installDebugHooks } from "./editor/boot/debugHooks";
import EditorApp from "./editor/boot/EditorApp";
import { FailurePage, failureOf } from "./editor/boot/ErrorBoundary";
import { reconcileUrl } from "./editor/boot/locale";
import { setBuildPhase } from "./editor/boot/phase";
import { loadUserNames } from "./editor/boot/userNames";
import { readLocale } from "./editor/state/prefs";
import { createRuntime } from "./editor/state/runtime";
import { installStyleNonce } from "./lib/cspNonce";
import { I18nProvider, type Message } from "./lib/i18n";

// Entry of the editor (SDD-012 section 2.3). The view applies the theme before
// paint; this file reads the bootstrap the route wrote into the page, builds
// the runtime and mounts the app.
// ?debug=1 installs the test hooks and the measures; &verify=1 freezes the
// documents and checks every applied diff against a fetched document (SDD-012
// sections 5.4 and 20.4). The end-to-end build (VITE_E2E=1) has both on.
const e2e = import.meta.env.VITE_E2E === "1";
const query = new URLSearchParams(location.search);
const debug = e2e || query.get("debug") === "1";
const verify = e2e || (debug && query.get("verify") === "1");
const mismatches: number[] = [];

function base(bootstrap: EditorBootstrap) {
	return {
		checked: verify,
		measure: debug,
		onMismatch: (revision: number) => {
			mismatches.push(revision);
		},
		projectId: bootstrap.project_id,
		actor: ownActor(bootstrap),
		mode: bootstrap.mode,
	};
}

async function main(): Promise<void> {
	const root = document.getElementById("editor");
	if (root === null) throw new Error("#editor is missing");

	// The language is a preference of this browser: when the page came back in
	// another one, load it again in the stored one (SDD-012 section 16.3).
	const lang = new URLSearchParams(location.search).get("lang");
	const again = reconcileUrl(
		document.documentElement.lang,
		readLocale(),
		location,
	);
	if (again !== null) {
		location.replace(again);
		return;
	}
	if (lang !== null) {
		const rest = new URLSearchParams(location.search);
		rest.delete("lang");
		const text = rest.toString();
		history.replaceState(
			null,
			"",
			location.pathname + (text === "" ? "" : `?${text}`),
		);
	}

	let bootstrap: EditorBootstrap;
	let runtime: ReturnType<typeof createRuntime>;

	const fixture = import.meta.env.DEV
		? new URLSearchParams(location.search).get("fixture")
		: null;
	if (fixture !== null) {
		const { devFixture } = await import("./editor/boot/devFixture");
		const dev = devFixture(Math.max(1, Number(fixture) || 4));
		bootstrap = dev.bootstrap;
		runtime = createRuntime({
			...base(bootstrap),
			...dev.options,
		});
	} else {
		bootstrap = parseBootstrap(
			document.getElementById("vv-bootstrap")?.textContent,
		);
		const transport = new HttpTransport(bootstrap.api_base);
		runtime = createRuntime({
			...base(bootstrap),
			transport,
			shareStream: true,
		});
		await loadUserNames(bootstrap, createClient(transport));
	}
	installStyleNonce();
	if (debug) installDebugHooks(runtime, mismatches);
	setBuildPhase(bootstrap.phase);

	document.documentElement.lang = bootstrap.locale;
	createRoot(root).render(
		<EditorApp bootstrap={bootstrap} runtime={runtime} />,
	);
	document.documentElement.removeAttribute("data-hydrating");
}

// A boot that throws before the app mounts (a bad bootstrap, a refused load of
// the user names) shows the error, not a blank page.
function bootFailure(error: unknown): void {
	console.error(error);
	const root = document.getElementById("editor");
	if (root === null) return;
	let messages: Record<string, Message> = {};
	let locale = "en";
	try {
		const parsed = parseBootstrap(
			document.getElementById("vv-bootstrap")?.textContent,
		);
		messages = parsed.messages;
		locale = parsed.locale;
	} catch {
		// The bootstrap is what failed: the keys show as they are.
	}
	createRoot(root).render(
		<I18nProvider messages={messages} locale={locale}>
			<FailurePage failure={failureOf(error)} />
		</I18nProvider>,
	);
	document.documentElement.removeAttribute("data-hydrating");
}

void main().catch(bootFailure);
