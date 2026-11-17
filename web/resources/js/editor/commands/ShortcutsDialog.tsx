import { Keyboard } from "lucide-react";
import { type ReactNode, useCallback, useEffect, useState } from "react";

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
import { useT } from "@/lib/i18n";
import { useRuntime } from "../state/EditorContext";
import { showTipsAgain } from "../state/tipsStore";
import {
	type BindingId,
	comboFromEvent,
	displayCombo,
	isBindingId,
	IS_MAC as MAC,
	SHORTCUTS,
} from "./keymap";
import { rebind, resetBindings, useBindings } from "./keymapStore";
import { useSurfaceRequest } from "./surface";

function isEditable(target: EventTarget | null): boolean {
	return (
		target instanceof HTMLElement &&
		(target.isContentEditable ||
			["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName))
	);
}

interface Conflict {
	id: BindingId;
	key: string;
	// A row id, or "reserved".
	with: string;
}

// The cheat sheet of SDD-012 section 18.2: one row per entry of SHORTCUTS, with
// the key that is current now. A rebindable row has a button that captures the
// next key press; a key that another row holds is refused and named.
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
	const bindings = useBindings();
	const [capturing, setCapturing] = useState<BindingId | null>(null);
	const [conflict, setConflict] = useState<Conflict | null>(null);

	useEffect(() => {
		if (!open) {
			setCapturing(null);
			setConflict(null);
		}
	}, [open]);

	const label = (id: string): string => t(`ui.editor.shortcuts.rows.${id}`);

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="max-w-xl" data-shortcuts="">
				<DialogHeader>
					<DialogTitle>{t("ui.editor.shortcuts.title")}</DialogTitle>
					<DialogDescription>
						{t("ui.editor.shortcuts.description")}
					</DialogDescription>
				</DialogHeader>
				<div className="max-h-[60vh] overflow-auto">
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
							{SHORTCUTS.map((row) => {
								const id = row.id;
								const shown = displayCombo(
									isBindingId(id) ? bindings[id] : row.keys,
									MAC,
								);
								return (
									<tr
										key={id}
										data-shortcut-row={id}
										className="border-t border-border"
									>
										<td className="py-1.5 pr-3">{label(id)}</td>
										<td className="py-1.5">
											{isBindingId(id) ? (
												<Button
													type="button"
													size="xs"
													variant="outline"
													data-shortcut-key={id}
													aria-label={t("ui.editor.shortcuts.change", {
														action: label(id),
														key: shown,
													})}
													onClick={() => {
														setConflict(null);
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
											) : (
												<kbd className="font-sans">{shown}</kbd>
											)}
											{row.aliases !== undefined &&
												(!isBindingId(id) || bindings[id] === row.keys) && (
													<span className="ml-2 text-xs text-muted-foreground">
														{t("ui.editor.shortcuts.also", {
															keys: displayCombo(row.aliases, MAC),
														})}
													</span>
												)}
										</td>
									</tr>
								);
							})}
						</tbody>
					</table>
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
							resetBindings();
						}}
					>
						{t("ui.editor.shortcuts.reset")}
					</Button>
					<Button
						type="button"
						variant="outline"
						onClick={() => {
							showTipsAgain();
							onOpenChange(false);
						}}
					>
						{t("ui.editor.tips.show_again")}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	);
}

// The top-bar entry to the cheat sheet and the `?` key that opens it outside
// an editable field or another dialog.
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
				event.key !== "?" ||
				event.ctrlKey ||
				event.metaKey ||
				event.altKey ||
				isEditable(event.target)
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
