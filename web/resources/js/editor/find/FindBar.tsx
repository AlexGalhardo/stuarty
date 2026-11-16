import { Search, X } from "lucide-react";
import {
	type KeyboardEvent as ReactKeyboardEvent,
	type ReactNode,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useT } from "@/lib/i18n";
import { isPaletteShortcut } from "../commands/keymap";
import { useDocumentState, useRuntime } from "../state/EditorContext";
import { revealItem } from "../timeline/revealItem";
import { type FindMatch, findMatches, stepIndex } from "./findMatches";
import { isFindShortcut } from "./findShortcut";

const MAX_ROWS = 500;

function isEditable(target: EventTarget | null): boolean {
	if (!(target instanceof HTMLElement)) return false;
	return (
		target.isContentEditable ||
		["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)
	);
}

function inDialog(target: EventTarget | null): boolean {
	return (
		target instanceof HTMLElement &&
		target.closest('[role="dialog"], [role="alertdialog"], [role="menu"]') !==
			null
	);
}

// The find bar of VV-052 (SDD-012 section 18.8): Mod+F opens it, in the browser
// and in the desktop window alike (the shell turns the accelerators of the web
// view off, so the page has to own the key). It looks through the names, the
// text and the caption words of the open project and moves the selection and the
// playhead; it sends nothing to the core.
export default function FindBar(): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	const document_ = useDocumentState((s) => s.document);
	const [open, setOpen] = useState(false);
	const [query, setQuery] = useState("");
	const [current, setCurrent] = useState(-1);
	const input = useRef<HTMLInputElement>(null);
	const root = useRef<HTMLElement>(null);
	const returnTo = useRef<HTMLElement | null>(null);

	const matches = useMemo(
		() => (open ? findMatches(document_, query) : []),
		[open, document_, query],
	);
	const at = current < matches.length ? current : -1;

	useEffect(() => {
		const onKey = (event: KeyboardEvent): void => {
			if (!isFindShortcut(event)) return;
			const inside = root.current?.contains(event.target as Node) ?? false;
			if (!inside && (isEditable(event.target) || inDialog(event.target))) {
				return;
			}
			event.preventDefault();
			if (!inside) {
				returnTo.current =
					window.document.activeElement instanceof HTMLElement
						? window.document.activeElement
						: null;
			}
			setOpen(true);
			requestAnimationFrame(() => {
				input.current?.focus();
				input.current?.select();
			});
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	}, []);

	function close(): void {
		setOpen(false);
		setQuery("");
		setCurrent(-1);
		const back = returnTo.current;
		returnTo.current = null;
		requestAnimationFrame(() => {
			if (back?.isConnected) back.focus();
			else
				window.document
					.querySelector<HTMLElement>('[data-landmark="4"]')
					?.focus();
		});
	}

	function kindLabel(kind: FindMatch["kind"]): string {
		if (kind === "name") return t("ui.editor.find.kind_name");
		if (kind === "text") return t("ui.editor.find.kind_text");
		return t("ui.editor.find.kind_word");
	}

	function go(index: number): void {
		const match: FindMatch | undefined = matches[index];
		if (match === undefined) return;
		setCurrent(index);
		runtime.uiWriter.select([match.itemId]);
		runtime.uiWriter.selectWord(
			match.wordIndex === undefined
				? null
				: { item: match.itemId, index: match.wordIndex },
		);
		runtime.uiWriter.setPlayhead(match.frame);
		revealItem(match.itemId);
	}

	function onKeyDown(event: ReactKeyboardEvent<HTMLElement>): void {
		if (event.key === "Escape") {
			event.preventDefault();
			event.stopPropagation();
			close();
			return;
		}
		if (event.key === "Enter" && event.target === input.current) {
			event.preventDefault();
			go(stepIndex(at, matches.length, event.shiftKey ? -1 : 1));
		}
		// The keys of the editor must not reach the timeline from here.
		if (!isPaletteShortcut(event.nativeEvent) && !isFindShortcut(event)) {
			event.nativeEvent.stopPropagation();
		}
	}

	if (!open) return null;
	return (
		<search
			ref={root}
			aria-label={t("ui.editor.find.label")}
			onKeyDown={onKeyDown}
			className="fixed top-24 right-4 z-40 flex w-80 flex-col gap-2 rounded-lg border border-ed-track-line bg-ed-panel p-2 text-sm text-foreground shadow-lg"
		>
			<div className="flex items-center gap-1">
				<Search aria-hidden="true" className="size-4 shrink-0" />
				<Input
					ref={input}
					type="text"
					role="searchbox"
					value={query}
					aria-label={t("ui.editor.find.label")}
					placeholder={t("ui.editor.find.placeholder")}
					onChange={(event) => {
						setQuery(event.target.value);
						setCurrent(-1);
					}}
				/>
				<Button
					type="button"
					size="icon-sm"
					variant="ghost"
					aria-label={t("ui.editor.find.close")}
					onClick={close}
				>
					<X aria-hidden="true" />
				</Button>
			</div>
			<p role="status" className="text-xs text-muted-foreground">
				{query.trim() === ""
					? t("ui.editor.find.hint")
					: matches.length === 0
						? t("ui.editor.find.none")
						: at < 0
							? t("ui.editor.find.found", { count: matches.length })
							: t("ui.editor.find.position", {
									current: at + 1,
									count: matches.length,
								})}
			</p>
			{matches.length > 0 && (
				<ul
					aria-label={t("ui.editor.find.results")}
					className="max-h-48 overflow-auto"
				>
					{matches.slice(0, MAX_ROWS).map((match, index) => (
						<li key={`${match.itemId}:${match.kind}:${match.wordIndex ?? ""}`}>
							<button
								type="button"
								aria-current={index === at ? "true" : undefined}
								onClick={() => go(index)}
								className="flex w-full items-baseline gap-2 rounded px-2 py-1 text-left hover:bg-muted aria-[current=true]:bg-muted aria-[current=true]:font-semibold"
							>
								<span className="shrink-0 text-xs text-muted-foreground">
									{kindLabel(match.kind)}
								</span>
								<span className="truncate">
									<UntrustedText>{match.label}</UntrustedText>
								</span>
							</button>
						</li>
					))}
				</ul>
			)}
		</search>
	);
}
