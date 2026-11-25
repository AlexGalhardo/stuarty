import { Head } from "@inertiajs/react";
import { type ReactNode, useMemo } from "react";

import { useT } from "@/lib/i18n";
import { HttpTransport } from "../../editor/api/http";
import Teleprompter from "../../editor/teleprompter/Teleprompter";
import { writingClient } from "../Write/client";

// The teleprompter of a project in a page of its own (SDD-025 section 20.3): what the desktop shell opens in a
// separate window that a screen capture does not hold. The controller sends only the id of the project; the page
// reads the script through the API and draws it with no sidebar, so the window is all text.
export default function Show({ projectId }: { projectId: string }): ReactNode {
	const t = useT();
	const client = useMemo(() => writingClient(new HttpTransport("/api/v1")), []);

	return (
		<>
			<Head title={t("ui.editor.teleprompter.title")} />
			<main className="flex h-dvh flex-col p-3">
				<Teleprompter
					client={client}
					projectId={projectId}
					className="flex-1"
				/>
			</main>
		</>
	);
}
