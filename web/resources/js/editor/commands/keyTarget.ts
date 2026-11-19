// Where a key press is aimed, shared by every shortcut handler (SDD-012
// section 18.9 item 2): a field keeps its own keys, and a dialog or a menu
// keeps the keys aimed at it.

export function isEditable(target: EventTarget | null): boolean {
	if (!(target instanceof HTMLElement)) return false;
	return (
		target.isContentEditable ||
		["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)
	);
}

// A key aimed at an open dialog or menu is not for the editor behind it.
export function inOverlay(target: EventTarget | null): boolean {
	return (
		target instanceof HTMLElement &&
		target.closest('[role="dialog"], [role="alertdialog"], [role="menu"]') !==
			null
	);
}
