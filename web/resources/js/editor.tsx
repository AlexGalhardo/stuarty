import { createRoot } from "react-dom/client";

import "../css/app.css";
import { HttpTransport } from "./editor/api/http";
import {
	type EditorBootstrap,
	ownActor,
	parseBootstrap,
} from "./editor/boot/bootstrap";
import EditorApp from "./editor/boot/EditorApp";
import { createRuntime } from "./editor/state/runtime";

// Entry of the editor (SDD-012 section 2.3). The view applies the theme before
// paint; this file reads the bootstrap the route wrote into the page, builds
// the runtime and mounts the app.
function base(bootstrap: EditorBootstrap) {
	return {
		projectId: bootstrap.project_id,
		actor: ownActor(bootstrap),
		mode: bootstrap.mode,
	};
}

async function main(): Promise<void> {
	const root = document.getElementById("editor");
	if (root === null) throw new Error("#editor is missing");

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
		runtime = createRuntime({
			...base(bootstrap),
			transport: new HttpTransport(bootstrap.api_base),
		});
	}

	document.documentElement.lang = bootstrap.locale;
	createRoot(root).render(
		<EditorApp bootstrap={bootstrap} runtime={runtime} />,
	);
	document.documentElement.removeAttribute("data-hydrating");
}

void main();
