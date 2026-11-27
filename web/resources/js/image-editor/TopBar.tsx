import { Link } from "@inertiajs/react";
import {
	ArrowLeft,
	CircleCheck,
	History,
	MessagesSquare,
	Search,
} from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useLocale, useT } from "@/lib/i18n";
import NotificationBell from "../shell/NotificationBell";
import { useShell } from "../shell/ShellContext";
import type { ImageDocument } from "./api/types";
import { AttachProjectTool } from "./export/AttachProjectTool";
import { ExportTool } from "./export/ExportDialog";
import { paletteHint } from "./palette/CommandPalette";
import { OperationTools } from "./toolbar/OperationTools";
import { useOperationDialogs } from "./toolbar/operations";
import { Toolbar } from "./toolbar/Toolbar";

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
		<span className="text-xs text-muted-foreground" data-saved-at="">
			{t("ui.image.editor.saved_auto", { time })}
		</span>
	);
}

// The Chat button of the shell, in the bar of the editor: it opens the same panel of the right dock.
function ChatButton(): ReactNode {
	const t = useT();
	const { chatOpen, openChat, chatButton } = useShell();
	return (
		<Button
			ref={chatButton}
			type="button"
			variant="outline"
			size="sm"
			className="h-8 px-3 font-semibold"
			aria-label={t("ui.chat.open")}
			aria-expanded={chatOpen}
			aria-controls="chat-panel"
			onClick={openChat}
			data-testid="chat-toggle"
		>
			<MessagesSquare aria-hidden="true" className="size-4" />
			{t("ui.shell.chat")}
		</Button>
	);
}

// The Commands button of the top bar: it opens the command palette, which Ctrl K (Cmd K on macOS) opens too.
function PaletteButton(): ReactNode {
	const t = useT();
	const { setPaletteOpen } = useOperationDialogs();
	const hint = paletteHint();
	return (
		<Button
			type="button"
			variant="ghost"
			size="sm"
			className="h-8 font-semibold text-text"
			aria-keyshortcuts={hint === "Ctrl K" ? "Control+K" : "Meta+K"}
			onClick={() => setPaletteOpen(true)}
			data-testid="image-palette-open"
		>
			<Search aria-hidden="true" className="size-4" />
			{t("ui.image.palette.open")}
			<kbd className="font-sans text-xs font-semibold text-muted-foreground">
				{hint}
			</kbd>
		</Button>
	);
}

// The top bar of the final design (SDD-012 section 24): the way back, the name of the image, its revision in a
// badge, its size and the saved check, and at the right the bell, Chat and the primary Export.
export default function TopBar({
	doc,
	failed = false,
	live = false,
}: {
	doc: ImageDocument | null;
	failed?: boolean;
	live?: boolean;
}): ReactNode {
	const t = useT();
	return (
		<header className="flex min-h-12 shrink-0 flex-wrap items-center gap-x-3 gap-y-1 border-b border-border bg-ed-panel px-3 py-2 lg:h-12 lg:flex-nowrap lg:py-0">
			{/* The mark of the product, as the sidebar of the shell draws it (dashboard/Sidebar.tsx). */}
			<img
				src="/icon.png"
				alt=""
				data-testid="image-app-mark"
				className="size-6 shrink-0"
			/>
			<Button asChild variant="ghost" size="icon-sm">
				<Link href="/edit-image" aria-label={t("ui.image.editor.back")}>
					<ArrowLeft aria-hidden="true" />
				</Link>
			</Button>
			<h1 id="image-title" className="min-w-0 truncate text-sm font-semibold">
				{doc === null ? (
					t(failed ? "ui.image.editor.title" : "ui.image.editor.loading")
				) : (
					<UntrustedText>{doc.name}</UntrustedText>
				)}
			</h1>
			{doc !== null && (
				<>
					<Badge
						variant="secondary"
						className="border-0 bg-surface-hover tabular-nums"
					>
						<History aria-hidden="true" />
						{t("ui.image.editor.rev_short", { revision: doc.revision })}
					</Badge>
					{live && (
						<Badge
							variant="secondary"
							className="bg-success-soft text-success-text"
							data-testid="image-live"
						>
							<CircleCheck aria-hidden="true" />
							{t("ui.editor.connection.live")}
						</Badge>
					)}
				</>
			)}
			<SavedIndicator revision={doc?.revision ?? null} />
			<span className="ml-auto" />
			<Toolbar
				label={t("ui.image.toolbar.history")}
				testId="image-history-toolbar"
				className="p-0"
			>
				<OperationTools group="history" />
			</Toolbar>
			<PaletteButton />
			<NotificationBell />
			<ChatButton />
			<AttachProjectTool />
			<ExportTool />
		</header>
	);
}
