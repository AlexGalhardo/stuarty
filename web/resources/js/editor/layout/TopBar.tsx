import {
	ArrowLeft,
	Loader,
	Redo2,
	RefreshCw,
	Undo2,
	Wifi,
	WifiOff,
} from "lucide-react";
import type { ReactNode } from "react";

import ThemeToggle from "@/components/ThemeToggle";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import ActivityIndicator from "./ActivityIndicator";

type Shown =
	| "connecting"
	| "live"
	| "reconnecting"
	| "offline"
	| "resyncing"
	| "failed";

const TONE: Record<Shown, string> = {
	connecting: "text-muted-foreground",
	live: "text-muted-foreground",
	reconnecting: "text-warning",
	offline: "text-destructive",
	resyncing: "text-info",
	failed: "text-destructive",
};

const ICON: Record<Shown, typeof Wifi> = {
	connecting: Loader,
	live: Wifi,
	reconnecting: RefreshCw,
	offline: WifiOff,
	resyncing: RefreshCw,
	failed: WifiOff,
};

// The top bar of SDD-012 section 6.2, Prototype subset: back link, project
// name (read only), revision and connection state as text and icon, undo and
// redo, the activity indicator and the theme toggle.
export default function TopBar(): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const name = useDocumentState((s) => s.document?.name);
	const revision = useDocumentState((s) => s.revision);
	const status = useDocumentState((s) => s.status);
	const connection = useDocumentState((s) => s.connection);

	const shown: Shown =
		status === "failed"
			? "failed"
			: status === "resyncing"
				? "resyncing"
				: status === "offline"
					? "offline"
					: connection;
	const Icon = ICON[shown];
	const ready = status === "ready" || status === "offline";

	return (
		<header className="col-span-full flex h-12 items-center gap-3 border-b border-ed-track-line bg-ed-panel px-3">
			<Button asChild variant="ghost" size="icon-sm">
				<a href="/projects" aria-label={t("ui.editor.back")}>
					<ArrowLeft aria-hidden="true" />
				</a>
			</Button>
			<h1 className="max-w-64 truncate text-sm font-semibold">
				{name !== undefined && <UntrustedText>{name}</UntrustedText>}
			</h1>
			<span
				className={cn("flex items-center gap-1.5 text-xs", TONE[shown])}
				data-connection={shown}
			>
				<Icon aria-hidden="true" className="size-3.5" />
				{revision !== null && (
					<span className="font-mono tabular-nums">
						{t("ui.editor.revision", { revision })}
					</span>
				)}
				<span>{t(`ui.editor.connection.${shown}`)}</span>
			</span>
			<div className="flex items-center gap-1">
				<Button
					type="button"
					variant="ghost"
					size="sm"
					disabled={!ready}
					onClick={() => void runtime.edits.undo()}
				>
					<Undo2 aria-hidden="true" />
					{t("ui.editor.undo")}
				</Button>
				<Button
					type="button"
					variant="ghost"
					size="sm"
					disabled={!ready}
					onClick={() => void runtime.edits.redo()}
				>
					<Redo2 aria-hidden="true" />
					{t("ui.editor.redo")}
				</Button>
			</div>
			<div className="ml-auto flex items-center gap-2">
				<ActivityIndicator />
				<ThemeToggle />
			</div>
		</header>
	);
}
