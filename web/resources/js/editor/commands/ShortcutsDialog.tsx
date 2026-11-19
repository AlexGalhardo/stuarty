import { Keyboard } from "lucide-react";
import {
	type ReactNode,
	useCallback,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";

import LocaleSelect from "@/components/LocaleSelect";
import ThemeToggle from "@/components/ThemeToggle";
import { Button } from "@/components/ui/button";
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { useT } from "@/lib/i18n";
import { useRuntime } from "../state/EditorContext";
import { startTour } from "../state/tourStore";
import {
	aliasKeys,
	comboFromEvent,
	displayCombo,
	IS_MAC as MAC,
	PRESET_IDS,
	type PresetId,
	SHORTCUTS,
} from "./keymap";
import {
	keysOf,
	rebind,
	resetBindings,
	rowFor,
	setPreset,
	useKeymap,
} from "./keymapStore";
import { isEditable } from "./keyTarget";
import { useSurfaceRequest } from "./surface";

interface Conflict {
	id: string;
	key: string;
	// A row id, or "reserved".
	with: string;
}

// The text of the keys a row answers to now, as shown ("Ctrl+B"). A row the
// person cannot rebind (the wheel, the grid keys) shows its own text.
function shownKeys(keys: readonly string[]): string {
	return keys.map((key) => displayCombo(key, MAC)).join(", ");
}

// The cheat sheet of SDD-012 sections 18.2 and 18.9: one row per entry of
// SHORTCUTS, with the key that is current now, a search field and the preset.
// Every row that is not fixed has a button that captures the next key press; a
// key that another row holds is refused and named.
export function ShortcutsDialog({
	open,
	onOpenChange,
	onLocaleChange,
}: {
	open: boolean;
	onOpenChange(open: boolean): void;
	onLocaleChange(locale: string): void;
}): ReactNode {
	const t = useT();
	const runtime = useRuntime();
	// The version is the service's, read when the dialog opens (SDD-012 section
	// 18.2); it is left out when the read fails.
	const [version, setVersion] = useState<string | null>(null);
	useEffect(() => {
		if (!open) return;
		let current = true;
		runtime.client.getSystem().then(
			(system) => {
				if (current) setVersion(system.version);
			},
			() => undefined,
		);
		return () => {
			current = false;
		};
	}, [open, runtime]);
	const keymap = useKeymap();
	const [capturing, setCapturing] = useState<string | null>(null);
	const [conflict, setConflict] = useState<Conflict | null>(null);
	const [dropped, setDropped] = useState<string[]>([]);
	const [query, setQuery] = useState("");
	const search = useRef<HTMLInputElement>(null);

	useEffect(() => {
		if (!open) {
			setCapturing(null);
			setConflict(null);
			setDropped([]);
			setQuery("");
			return;
		}
		const frame = requestAnimationFrame(() => search.current?.focus());
		return () => cancelAnimationFrame(frame);
	}, [open]);

	const label = useCallback(
		(id: string): string => t(`ui.editor.shortcuts.rows.${id}`),
		[t],
	);

	const rows = useMemo(() => {
		const needle = query.trim().toLowerCase();
		if (needle === "") return SHORTCUTS;
		return SHORTCUTS.filter((row) => {
			const keys = shownKeys(
				row.fixed === true ? [row.keys] : keysOf(row, keymap),
			);
			return (
				label(row.id).toLowerCase().includes(needle) ||
				keys.toLowerCase().includes(needle)
			);
		});
	}, [query, keymap, label]);

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="max-w-2xl" data-shortcuts="">
				<DialogHeader>
					<DialogTitle>{t("ui.editor.shortcuts.title")}</DialogTitle>
					<DialogDescription>
						{t("ui.editor.shortcuts.description")}
					</DialogDescription>
				</DialogHeader>
				<div className="flex flex-wrap items-center gap-2">
					<Input
						ref={search}
						type="search"
						value={query}
						aria-label={t("ui.editor.shortcuts.search")}
						placeholder={t("ui.editor.shortcuts.search")}
						className="min-w-48 flex-1"
						onChange={(event) => setQuery(event.target.value)}
						onKeyDown={(event) => {
							// Esc clears a search first and closes the dialog second.
							if (event.key === "Escape" && query !== "") {
								event.preventDefault();
								event.stopPropagation();
								setQuery("");
							}
						}}
					/>
					<label className="flex items-center gap-2 text-sm">
						<span className="text-muted-foreground">
							{t("ui.editor.shortcuts.preset")}
						</span>
						<select
							value={keymap.preset}
							data-shortcuts-preset=""
							className="h-8 rounded-md border border-input bg-transparent px-2 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30"
							onChange={(event) => {
								setConflict(null);
								setDropped(setPreset(event.target.value as PresetId).dropped);
							}}
						>
							{PRESET_IDS.map((id) => (
								<option key={id} value={id}>
									{t(`ui.editor.shortcuts.presets.${id}`)}
								</option>
							))}
						</select>
					</label>
				</div>
				<div className="max-h-[50vh] overflow-auto">
					<table className="w-full text-sm">
						<thead>
							<tr className="text-left text-xs text-muted-foreground">
								<th scope="col" className="py-1 pr-3 font-medium">
									{t("ui.editor.shortcuts.action")}
								</th>
								<th scope="col" className="py-1 font-medium">
									{t("ui.editor.shortcuts.key")}
								</th>
							</tr>
						</thead>
						<tbody>
							{rows.map((row) => {
								const id = row.id;
								const fixed = row.fixed === true;
								const current = fixed ? [row.keys] : keysOf(row, keymap);
								const shown = shownKeys(current);
								const own = keymap.overrides[id] === undefined;
								const aliases = own ? aliasKeys(row) : [];
								return (
									<tr
										key={id}
										data-shortcut-row={id}
										className="border-t border-border"
									>
										<td className="py-1.5 pr-3">{label(id)}</td>
										<td className="py-1.5">
											{fixed ? (
												<kbd className="font-sans">{shown}</kbd>
											) : (
												<Button
													type="button"
													size="xs"
													variant="outline"
													data-shortcut-key={id}
													// While capturing, the name says so: a label that
													// kept naming the old key would hide the prompt from
													// a screen reader.
													aria-label={
														capturing === id
															? t("ui.editor.shortcuts.press_key")
															: t("ui.editor.shortcuts.change", {
																	action: label(id),
																	key: shown,
																})
													}
													onClick={() => {
														setConflict(null);
														setDropped([]);
														setCapturing(id);
													}}
													onBlur={() => setCapturing(null)}
													onKeyDown={(event) => {
														if (capturing !== id) return;
														event.preventDefault();
														event.stopPropagation();
														if (event.key === "Escape") {
															setCapturing(null);
															return;
														}
														const combo = comboFromEvent(event.nativeEvent);
														if (combo === null) return;
														const result = rebind(id, combo);
														setCapturing(null);
														setConflict(
															result.ok
																? null
																: { id, key: combo, with: result.conflict },
														);
													}}
												>
													{capturing === id ? (
														t("ui.editor.shortcuts.press_key")
													) : (
														<kbd className="font-sans">{shown}</kbd>
													)}
												</Button>
											)}
											{aliases.length > 0 && (
												<span className="ml-2 text-xs text-muted-foreground">
													{t("ui.editor.shortcuts.also", {
														keys: displayCombo(aliases.join(", "), MAC),
													})}
												</span>
											)}
										</td>
									</tr>
								);
							})}
						</tbody>
					</table>
					{rows.length === 0 && (
						<p
							role="status"
							className="py-6 text-center text-sm text-muted-foreground"
						>
							{t("ui.editor.shortcuts.none")}
						</p>
					)}
				</div>
				{conflict !== null && (
					<p role="alert" className="text-sm text-destructive">
						{conflict.with === "reserved"
							? t("ui.editor.shortcuts.reserved", {
									key: displayCombo(conflict.key, MAC),
								})
							: t("ui.editor.shortcuts.conflict", {
									key: displayCombo(conflict.key, MAC),
									action: label(conflict.with),
								})}
					</p>
				)}
				{dropped.length > 0 && (
					<p role="status" className="text-sm text-muted-foreground">
						{t("ui.editor.shortcuts.dropped", {
							actions: dropped.map(label).join(", "),
						})}
					</p>
				)}
				<div className="flex items-center justify-between gap-3 border-t pt-3">
					<div className="flex items-center gap-2">
						<LocaleSelect onChange={onLocaleChange} />
						<ThemeToggle />
					</div>
					{version !== null && (
						<span className="text-xs text-muted-foreground" data-version="">
							{t("ui.editor.shortcuts.version", { version })}
						</span>
					)}
				</div>
				<DialogFooter>
					<Button
						type="button"
						variant="outline"
						onClick={() => {
							setConflict(null);
							setDropped([]);
							resetBindings();
						}}
					>
						{t("ui.editor.shortcuts.reset")}
					</Button>
					<Button
						type="button"
						variant="outline"
						onClick={() => {
							startTour();
							onOpenChange(false);
						}}
					>
						{t("ui.editor.tour.replay")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

// The top-bar entry to the cheat sheet and the `cheat_sheet` row that opens it
// outside an editable field or another dialog.
export default function ShortcutsButton({
	onLocaleChange,
}: {
	onLocaleChange(locale: string): void;
}): ReactNode {
	const t = useT();
	const [open, setOpen] = useState(false);
	const openFromMenu = useCallback(() => setOpen(true), []);
	useSurfaceRequest("cheat_sheet", openFromMenu);

	useEffect(() => {
		const onKey = (event: KeyboardEvent): void => {
			if (
				isEditable(event.target) ||
				rowFor(event, ["global"]) !== "cheat_sheet"
			)
				return;
			const target = event.target;
			const inOther =
				target instanceof HTMLElement &&
				target.closest(
					'[role="dialog"]:not([data-shortcuts]), [role="alertdialog"], [role="menu"]',
				) !== null;
			if (inOther) return;
			event.preventDefault();
			setOpen((was) => !was);
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	}, []);

	return (
		<>
			<Button
				type="button"
				size="icon-sm"
				variant="ghost"
				aria-label={t("ui.editor.shortcuts.open")}
				title={t("ui.editor.shortcuts.open")}
				onClick={() => setOpen(true)}
			>
				<Keyboard aria-hidden="true" />
			</Button>
			<ShortcutsDialog
				open={open}
				onOpenChange={setOpen}
				onLocaleChange={onLocaleChange}
			/>
		</>
	);
}
