import {
	ArrowLeft,
	Check,
	History,
	Loader,
	MessagesSquare,
	Pencil,
	RefreshCw,
	Wifi,
	WifiOff,
} from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import LocaleSelect from "@/components/LocaleSelect";
import ThemeToggle from "@/components/ThemeToggle";
import UntrustedText from "@/components/UntrustedText";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/cn";
import { useLocale, useT } from "@/lib/i18n";
import NotificationBell from "../../shell/NotificationBell";
import { useShell } from "../../shell/ShellContext";
import PaletteButton from "../commands/palette/PaletteButton";
import ShortcutsButton from "../commands/ShortcutsDialog";
import { useActions, useCanEdit } from "../commands/useActions";
import ExportButton from "../export/ExportButton";
import { useDocumentState } from "../state/EditorContext";
import ActivityIndicator from "./ActivityIndicator";
import CompositionBreadcrumb from "./CompositionBreadcrumb";
import EditorMenu from "./EditorMenu";
import { useOpenEditorChat } from "./EditorSceneBar";

type Shown =
	| "connecting"
	| "live"
	| "reconnecting"
	| "offline"
	| "resyncing"
	| "failed";

const TONE: Record<Shown, string> = {
	connecting: "bg-surface-sunken text-muted-foreground",
	live: "bg-success-soft text-success-text",
	reconnecting: "bg-warning-soft text-warning-text",
	offline: "bg-danger-soft text-danger-text",
	resyncing: "bg-surface-sunken text-info",
	failed: "bg-danger-soft text-danger-text",
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
	const text = t("ui.editor.saved_auto", { time });
	return (
		<span
			className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground"
			data-saved-at=""
			title={text}
		>
			<Check aria-hidden="true" className="size-3.5 shrink-0 text-success" />
			<span className="truncate">{text}</span>
		</span>
	);
}

// The Chat button of the bar: it opens the same panel as the scene bar, over the editor (VV-427).
function ChatButton(): ReactNode {
	const t = useT();
	const { chatOpen } = useShell();
	const open = useOpenEditorChat();
	return (
		<Button
			type="button"
			variant="outline"
			size="sm"
			aria-label={t("ui.chat.open")}
			aria-expanded={chatOpen}
			aria-controls="chat-panel"
			onClick={open}
			data-testid="chat-toggle"
		>
			<MessagesSquare aria-hidden="true" />
			{t("ui.shell.chat")}
		</Button>
	);
}

// The agent part of the bar, drawn only when the page has a session (the bell and Chat read the shell).
function AgentActions(): ReactNode {
	return (
		<>
			<NotificationBell />
			<ChatButton />
		</>
	);
}

// The top bar of the final design (SDD-012 section 6.2): the menu, the way back, the name of the project (click to
// rename), its revision and its connection in badges, the saved check, and at the right the commands, the bell,
// Chat and the primary Export. The activity indicator, the shortcuts, the language and the theme stay in the bar.
export default function TopBar({
	onLocaleChange,
	agent = false,
}: {
	onLocaleChange(locale: string): void;
	// the page has a session of the agent: the bell and Chat are drawn
	agent?: boolean;
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
			<a
				href="/"
				aria-label={t("ui.nav.dashboard")}
				data-testid="product-mark"
				className="flex size-8 shrink-0 items-center justify-center rounded-md focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
			>
				<img src="/icon.png" alt="" className="size-7" />
			</a>
			<Button asChild variant="ghost" size="icon-sm">
				<a href="/projects" aria-label={t("ui.editor.back")}>
					<ArrowLeft aria-hidden="true" />
				</a>
			</Button>
			<EditorMenu />
			<span aria-hidden="true" className="h-5 w-px bg-ed-track-line" />
			<CompositionBreadcrumb />
			{name !== undefined && <ProjectName name={name} />}
			{revision !== null && (
				<Badge variant="secondary" className="font-mono tabular-nums">
					<History aria-hidden="true" />
					{t("ui.editor.revision", { revision })}
				</Badge>
			)}
			<Badge
				variant="secondary"
				className={cn(TONE[shown])}
				data-connection={shown}
			>
				<Icon aria-hidden="true" />
				{t(`ui.editor.connection.${shown}`)}
			</Badge>
			<SavedIndicator live={shown === "live"} />
			<div className="ml-auto flex shrink-0 items-center gap-2">
				<ActivityIndicator />
				<PaletteButton />
				{agent && <AgentActions />}
				<ExportButton />
				<ShortcutsButton onLocaleChange={onLocaleChange} />
				<LocaleSelect onChange={onLocaleChange} />
				<ThemeToggle />
			</div>
		</header>
	);
}
