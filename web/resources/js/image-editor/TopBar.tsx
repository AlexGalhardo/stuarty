import { Link } from "@inertiajs/react";
import { ArrowLeft, Check } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { useLocale, useT } from "@/lib/i18n";
import type { ImageDocument } from "./api/types";

// "Saved automatically: 14:32": the core stores every edit before its answer arrives, so the time is the
// moment the open revision reached the screen. It says nothing until a document is on screen.
export function SavedIndicator({
	revision,
}: {
	revision: number | null;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const seen = useRef(revision);
	const [savedAt, setSavedAt] = useState<Date | null>(() =>
		revision === null ? null : new Date(),
	);
	useEffect(() => {
		if (revision === seen.current) return;
		seen.current = revision;
		if (revision !== null) setSavedAt(new Date());
	}, [revision]);
	if (savedAt === null) return null;
	const time = new Intl.DateTimeFormat(locale, {
		hour: "2-digit",
		minute: "2-digit",
	}).format(savedAt);
	return (
		<span
			className="flex items-center gap-1 text-xs text-muted-foreground"
			data-saved-at=""
		>
			<Check aria-hidden="true" className="size-3.5 text-success" />
			{t("ui.image.editor.saved_auto", { time })}
		</span>
	);
}

// The bar over the toolbar, as in the video editor (SDD-012 section 24): the way back, the name of the image,
// its size and revision as text, and the saved check at the right.
export default function TopBar({
	doc,
}: {
	doc: ImageDocument | null;
}): ReactNode {
	const t = useT();
	return (
		<header className="flex items-center gap-2 border-b border-border bg-ed-panel px-2 py-1">
			<Button asChild variant="ghost" size="icon-sm">
				<Link href="/edit-image" aria-label={t("ui.image.editor.back")}>
					<ArrowLeft aria-hidden="true" />
				</Link>
			</Button>
			<h1 id="image-title" className="min-w-0 truncate text-sm font-semibold">
				{doc === null ? (
					t("ui.image.editor.loading")
				) : (
					<UntrustedText>{doc.name}</UntrustedText>
				)}
			</h1>
			{doc !== null && (
				<span className="text-xs text-muted-foreground tabular-nums">
					{t("ui.image.editor.size", { w: doc.width, h: doc.height })}
					{" · "}
					{t("ui.image.editor.revision", { revision: doc.revision })}
				</span>
			)}
			<span className="ml-auto" />
			<SavedIndicator revision={doc?.revision ?? null} />
		</header>
	);
}
