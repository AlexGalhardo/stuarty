import type { WritingClient } from "../../pages/Write/client";

// The script the teleprompter shows (SDD-025 section 20.3): the newest active script of the project, read with the
// writing operations that exist. The body is untrusted text and is only ever drawn as text.
export interface ProjectScript {
	itemId: string;
	title: string;
	body: string;
}

export async function loadScript(
	client: Pick<WritingClient, "list" | "get">,
	projectId: string,
): Promise<ProjectScript | null> {
	const found = await client.list({
		project_id: projectId,
		text_kind: "script",
		state: "active",
	});
	// The service lists the newest first.
	const newest = found.texts[0];
	if (newest === undefined) return null;
	const read = await client.get(projectId, newest.id);
	return { itemId: newest.id, title: read.item.title, body: read.body };
}
