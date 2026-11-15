import type { ApiClient } from "../api/client";
import { setUserNames } from "../state/activityStore";
import type { EditorBootstrap } from "./bootstrap";

const WAIT_MS = 2000;

// Names for the actors `user:<id>` of a network workspace (SDD-012 section 5.8
// item 6). The own name comes with the bootstrap; the others come from
// `GET /users`, which only an administrator session may call, so for anyone
// else the list is refused and those actors keep showing as `user:<id>`. The
// wait is short and any failure is ignored: a name is a nicety, not a need.
export async function loadUserNames(
	bootstrap: EditorBootstrap,
	client: Pick<ApiClient, "listUsers">,
): Promise<void> {
	if (bootstrap.mode !== "network") return;
	const names: Record<string, string> = {};
	try {
		const list = await Promise.race([
			client.listUsers(),
			new Promise<null>((resolve) => setTimeout(() => resolve(null), WAIT_MS)),
		]);
		for (const user of list?.users ?? []) names[user.user_id] = user.email;
	} catch {
		// Not an administrator, or the server did not answer.
	}
	if (bootstrap.user !== null) names[bootstrap.user.id] = bootstrap.user.name;
	setUserNames(names);
}
