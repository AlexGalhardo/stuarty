import {
	ArrowLeft,
	House,
	Loader,
	Pencil,
	RefreshCw,
	Wifi,
	WifiOff,
} from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import LocaleSelect from "@/components/LocaleSelect";
import ThemeToggle from "@/components/ThemeToggle";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/cn";
import { useLocale, useT } from "@/lib/i18n";
import PaletteButton from "../commands/palette/PaletteButton";
import ShortcutsButton from "../commands/ShortcutsDialog";
import { useActions, useCanEdit } from "../commands/useActions";
import ExportButton from "../export/ExportButton";
import { useDocumentState } from "../state/EditorContext";
import ActivityIndicator from "./ActivityIndicator";
import EditorMenu from "./EditorMenu";

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

// The name of the project: a click opens the field, Enter or leaving it sends
// `project.rename`, Esc leaves it alone (SDD-012 section 6.2).
function ProjectName({ name }: { name: string }): ReactNode {
	const t = useT();
	const actions = useActions();
	const canEdit = useCanEdit();
	const [editing, setEditing] = useState(false);
	const finished = useRef(false);
	const finish = (value: string | null): void => {
		if (finished.current) return;
		finished.current = true;
		setEditing(false);
		// An empty name is refused by the core, so it is not sent.
		if (value !== null && value !== "" && value !== name)
			void actions.renameProject(value);
	};
	if (editing) {
		return (
			<Input
				autoFocus
				aria-label={t("ui.editor.project.name_label")}
				defaultValue={name}
				maxLength={120}
				className="h-7 w-64 text-sm font-semibold"
				onFocus={(event) => event.currentTarget.select()}
				onKeyDown={(event) => {
					event.stopPropagation();
					if (event.key === "Enter") finish(event.currentTarget.value);
					else if (event.key === "Escape") finish(null);
				}}
				onBlur={(event) => finish(event.currentTarget.value)}
			/>
		);
	}
	return (
		<>
			<h1 className="max-w-64 truncate text-sm font-semibold">
				<UntrustedText>{name}</UntrustedText>
			</h1>
			<Button
				type="button"
				variant="ghost"
				size="icon-sm"
				disabled={!canEdit}
				aria-label={t("ui.editor.project.rename", { name })}
				onClick={() => {
					finished.current = false;
					setEditing(true);
				}}
			>
				<Pencil aria-hidden="true" />
			</Button>
		</>
	);
}

// "Saved automatically: 14:32": every edit is already stored by the core when
// its revision arrives, so the time is the moment the open revision reached the
// editor. It stays mounted while the editor is not live, so a reconnect does
// not restart the time, and it says nothing then.
export function SavedIndicator({ live }: { live: boolean }): ReactNode {
	const t = useT();
	const locale = useLocale();
	const revision = useDocumentState((s) => s.revision);
	const seen = useRef(revision);
	const [savedAt, setSavedAt] = useState<Date | null>(() =>
		revision === null ? null : new Date(),
	);
	useEffect(() => {
		if (revision === seen.current) return;
		seen.current = revision;
		if (revision !== null) setSavedAt(new Date());
	}, [revision]);
	if (!live || savedAt === null) return null;
	const time = new Intl.DateTimeFormat(locale, {
		hour: "2-digit",
		minute: "2-digit",
	}).format(savedAt);
	return (
		<span className="text-xs text-muted-foreground" data-saved-at="">
			{t("ui.editor.saved_auto", { time })}
		</span>
	);
}

// The top bar of SDD-012 section 6.2: back link, project
// name (click to rename), revision and connection state as text and icon, undo and
// redo, the activity indicator and the theme toggle.
export default function TopBar({
	onLocaleChange,
}: {
	onLocaleChange(locale: string): void;
}): ReactNode {
	const t = useT();
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

	return (
		<header
			data-landmark="1"
			className="col-span-full flex h-12 items-center gap-3 border-b border-ed-track-line bg-ed-panel px-3"
		>
			<EditorMenu />
			<Button asChild variant="ghost" size="icon-sm">
				<a href="/projects" aria-label={t("ui.editor.back")}>
					<ArrowLeft aria-hidden="true" />
				</a>
			</Button>
			<Button asChild variant="ghost" size="icon-sm">
				<a href="/" aria-label={t("ui.nav.dashboard")}>
					<House aria-hidden="true" />
				</a>
			</Button>
			{name !== undefined && <ProjectName name={name} />}
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
			<SavedIndicator live={shown === "live"} />
			<div className="ml-auto flex items-center gap-2">
				<PaletteButton />
				<ShortcutsButton onLocaleChange={onLocaleChange} />
				<ExportButton />
				<ActivityIndicator />
				<LocaleSelect onChange={onLocaleChange} />
				<ThemeToggle />
			</div>
		</header>
	);
}
