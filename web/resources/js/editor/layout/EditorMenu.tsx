import { Menu } from "lucide-react";
import { type ReactNode, useSyncExternalStore } from "react";

import { Button } from "@/components/ui/button";
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuShortcut,
	DropdownMenuSub,
	DropdownMenuSubContent,
	DropdownMenuSubTrigger,
	DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { getDesktopBridge, subscribeDesktopBridge } from "@/lib/desktopBridge";
import { useT } from "@/lib/i18n";
import { IS_MAC, keyLabel } from "../commands/keymap";
import { useBindings } from "../commands/keymapStore";
import {
	MENUS,
	type MenuDef,
	type MenuEntry,
	type MenuItemDef,
} from "../commands/menu";
import { useCanCut, useCanEdit, useHasSelection } from "../commands/useActions";
import { useCommandNames } from "../commands/useExposedCommands";
import { useMenuRunner } from "../commands/useMenuRunner";

function useInDesktopShell(): boolean {
	return useSyncExternalStore(
		subscribeDesktopBridge,
		() => getDesktopBridge() !== null,
		() => false,
	);
}

// The items of a menu that the registry supports; separators next to a missing
// item or at an edge are dropped, so the menu never shows an empty group.
function visibleEntries(
	menu: MenuDef,
	names: ReadonlySet<string>,
): readonly MenuEntry[] {
	const drawn = menu.items.filter(
		(entry) =>
			entry === "separator" ||
			(entry.commands ?? []).every((name) => names.has(name)),
	);
	return drawn.filter(
		(entry, i) =>
			entry !== "separator" ||
			(i > 0 && i < drawn.length - 1 && drawn[i - 1] !== "separator"),
	);
}

// The "Menu" button of the top bar (SDD-012 section 6.4). Its items are the
// table of `commands/menu.ts`, the same one the native menu bar of the desktop
// shell is built from, so the button is not drawn there. Each item runs the same
// action as the control of the page, and its key is read from the cheat sheet.
export default function EditorMenu(): ReactNode {
	const t = useT();
	const inShell = useInDesktopShell();
	const names = useCommandNames();
	const bindings = useBindings();
	const run = useMenuRunner();
	const canEdit = useCanEdit();
	const hasSelection = useHasSelection();
	const canCut = useCanCut();
	if (inShell) return null;

	const enabled = (item: MenuItemDef): boolean =>
		item.need === undefined ||
		(item.need === "edit" && canEdit) ||
		(item.need === "selection" && hasSelection) ||
		(item.need === "cut" && canCut);

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild>
				<Button type="button" variant="ghost" size="sm">
					<Menu aria-hidden="true" />
					{t("ui.editor.menu.label")}
				</Button>
			</DropdownMenuTrigger>
			<DropdownMenuContent align="start" className="min-w-44">
				{MENUS.map((menu) => {
					const entries = visibleEntries(menu, names);
					if (entries.every((entry) => entry === "separator")) return null;
					return (
						<DropdownMenuSub key={menu.id}>
							<DropdownMenuSubTrigger>
								{t(menu.labelKey)}
							</DropdownMenuSubTrigger>
							<DropdownMenuSubContent className="min-w-56">
								{entries.map((entry, i) => {
									if (entry === "separator") {
										// biome-ignore lint/suspicious/noArrayIndexKey: a separator has no id and the list does not reorder.
										return <DropdownMenuSeparator key={`sep-${i}`} />;
									}
									const key =
										entry.row === undefined
											? null
											: keyLabel(entry.row, bindings, IS_MAC);
									return (
										<DropdownMenuItem
											key={entry.id}
											disabled={!enabled(entry)}
											onSelect={() => void run(entry.id)}
										>
											{t(entry.labelKey)}
											{key !== null && (
												<DropdownMenuShortcut>{key}</DropdownMenuShortcut>
											)}
										</DropdownMenuItem>
									);
								})}
							</DropdownMenuSubContent>
						</DropdownMenuSub>
					);
				})}
			</DropdownMenuContent>
		</DropdownMenu>
	);
}
