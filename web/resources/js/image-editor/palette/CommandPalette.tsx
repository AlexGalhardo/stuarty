import {
	Fragment,
	type KeyboardEvent,
	type ReactNode,
	useEffect,
	useId,
	useMemo,
	useRef,
	useState,
} from "react";

import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import {
	type Operation,
	type OperationGroup,
	useOperationDialogs,
	useOperations,
} from "../toolbar/operations";
import { SHORTCUTS } from "../toolbar/shortcuts";

const GROUP_LABEL: Record<OperationGroup, string> = {
	history: "ui.image.toolbar.history",
	add: "ui.image.toolbar.add",
	arrange: "ui.image.toolbar.arrange",
	model: "ui.image.toolbar.model",
	view: "ui.image.toolbar.view",
};

// Ctrl K on Windows and Linux, Cmd K on macOS.
export function isPaletteKey(event: {
	key: string;
	ctrlKey: boolean;
	metaKey: boolean;
	altKey: boolean;
	shiftKey: boolean;
}): boolean {
	return (
		(event.ctrlKey || event.metaKey) &&
		!event.altKey &&
		!event.shiftKey &&
		event.key.toLowerCase() === "k"
	);
}

// A key aimed at another dialog or a menu (the import, the model dialog, the export, "more") stays there: the
// palette opens over the editor, never as a second modal over one that is open. Inside the palette it closes it.
function aimedAtOtherOverlay(target: EventTarget | null): boolean {
	if (!(target instanceof Element)) return false;
	const overlay = target.closest(
		'[role="dialog"], [role="alertdialog"], [role="menu"]',
	);
	return overlay !== null && !overlay.hasAttribute("data-command-palette");
}

// The key the top bar shows beside "Commands".
export function paletteHint(): string {
	const mac =
		typeof navigator !== "undefined" &&
		/mac|iphone|ipad/i.test(navigator.platform);
	return mac ? "⌘ K" : "Ctrl K";
}

// The command palette (Ctrl K): a dialog with a search field over the operations the editor already offers, as a
// combobox with a listbox. Choosing one runs the same `run` of the same operation as its button, so it sends the
// same command; an operation that is not available now is listed dimmed and the arrows skip it.
export function CommandPalette(): ReactNode {
	const t = useT();
	const { paletteOpen, setPaletteOpen } = useOperationDialogs();
	const operations = useOperations();
	const [query, setQuery] = useState("");
	const [active, setActive] = useState(0);
	const listId = useId();
	const list = useRef<HTMLDivElement>(null);

	useEffect(() => {
		const onKey = (event: globalThis.KeyboardEvent): void => {
			if (!isPaletteKey(event) || aimedAtOtherOverlay(event.target)) return;
			event.preventDefault();
			setPaletteOpen(!paletteOpen);
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	}, [paletteOpen, setPaletteOpen]);

	useEffect(() => {
		if (paletteOpen) {
			setQuery("");
			setActive(0);
		}
	}, [paletteOpen]);

	const shown = useMemo(() => {
		const needle = query.trim().toLowerCase();
		return operations.filter(
			(o) =>
				needle === "" ||
				o.label.toLowerCase().includes(needle) ||
				t(GROUP_LABEL[o.group]).toLowerCase().includes(needle),
		);
	}, [operations, query, t]);

	// The first available row of the result is the one Enter runs until an arrow chooses another.
	const first = shown.findIndex((o) => !o.disabled);
	const current = shown[active]?.disabled === false ? active : first;

	useEffect(() => {
		if (current < 0) return;
		list.current
			?.querySelector('[aria-selected="true"]')
			?.scrollIntoView?.({ block: "nearest" });
	}, [current]);

	const run = (operation: Operation): void => {
		if (operation.disabled) return;
		setPaletteOpen(false);
		// After the dialog has closed, so a dialog the operation opens takes the focus the palette gave back.
		window.setTimeout(operation.run, 0);
	};

	const move = (direction: 1 | -1): void => {
		if (shown.length === 0) return;
		let at = current < 0 ? (direction === 1 ? -1 : 0) : current;
		for (let i = 0; i < shown.length; i += 1) {
			at = (at + direction + shown.length) % shown.length;
			if (shown[at]?.disabled === false) {
				setActive(at);
				return;
			}
		}
	};

	const onKeyDown = (event: KeyboardEvent<HTMLInputElement>): void => {
		if (event.key === "ArrowDown") {
			event.preventDefault();
			move(1);
		} else if (event.key === "ArrowUp") {
			event.preventDefault();
			move(-1);
		} else if (event.key === "Enter") {
			event.preventDefault();
			const target = shown[current];
			if (target !== undefined) run(target);
		}
	};

	return (
		<Dialog open={paletteOpen} onOpenChange={setPaletteOpen}>
			<DialogContent
				showCloseButton={false}
				data-testid="image-palette"
				data-command-palette=""
				className="top-[18%] translate-y-0 gap-0 p-0 sm:max-w-lg"
			>
				<DialogTitle className="sr-only">
					{t("ui.image.palette.title")}
				</DialogTitle>
				<DialogDescription className="sr-only">
					{t("ui.image.palette.description")}
				</DialogDescription>
				<div className="border-b border-border p-2">
					<Input
						role="combobox"
						aria-expanded="true"
						aria-controls={listId}
						aria-autocomplete="list"
						aria-activedescendant={
							current < 0 ? undefined : `${listId}-${shown[current]?.id}`
						}
						aria-label={t("ui.image.palette.placeholder")}
						placeholder={t("ui.image.palette.placeholder")}
						value={query}
						autoComplete="off"
						spellCheck={false}
						data-testid="image-palette-input"
						// One ring: the 2 px focus outline of the design system sits on the edge, with no second border.
						className="focus-visible:border-transparent"
						style={{ outlineOffset: 0 }}
						onChange={(event) => {
							setQuery(event.target.value);
							setActive(0);
						}}
						onKeyDown={onKeyDown}
					/>
				</div>
				<div
					ref={list}
					id={listId}
					role="listbox"
					aria-label={t("ui.image.palette.list")}
					className="max-h-80 overflow-y-auto p-1"
				>
					{shown.map((o, index) => {
						const Icon = o.icon;
						const keys =
							o.shortcut === undefined ? undefined : SHORTCUTS[o.shortcut];
						const heading = index === 0 || shown[index - 1]?.group !== o.group;
						return (
							<Fragment key={o.id}>
								{heading && (
									<div
										role="presentation"
										className="px-2 pt-2 pb-1 text-xs font-semibold text-text-muted"
									>
										{t(GROUP_LABEL[o.group])}
									</div>
								)}
								{/* The keys belong to the combobox: focus stays in the field, which runs the active option on Enter. */}
								{/* biome-ignore lint/a11y/useKeyWithClickEvents: keyboard handled by the combobox input */}
								<div
									id={`${listId}-${o.id}`}
									role="option"
									tabIndex={-1}
									aria-selected={index === current}
									aria-disabled={o.disabled || undefined}
									data-testid={`image-palette-${o.id}`}
									onClick={() => run(o)}
									className={cn(
										"flex cursor-default items-center gap-2 rounded-md px-2 py-1.5 text-sm",
										index === current && "bg-accent-soft text-text",
										o.disabled && "cursor-not-allowed opacity-50",
									)}
								>
									<Icon aria-hidden="true" className="size-4 shrink-0" />
									<span className="min-w-0 flex-1 truncate">{o.label}</span>
									{o.disabled && (
										<span className="text-xs text-muted-foreground">
											{t("ui.image.palette.unavailable")}
										</span>
									)}
									{keys !== undefined && (
										<kbd className="rounded-sm border border-border px-1 font-sans text-xs text-muted-foreground">
											{keys}
										</kbd>
									)}
								</div>
							</Fragment>
						);
					})}
				</div>
				{shown.length === 0 && (
					<p
						role="status"
						className="m-0 px-3 py-3 text-sm text-muted-foreground"
					>
						{t("ui.image.palette.empty")}
					</p>
				)}
				<p className="m-0 border-t border-border px-3 py-1.5 text-xs text-muted-foreground">
					{t("ui.image.palette.hint")}
				</p>
			</DialogContent>
		</Dialog>
	);
}
