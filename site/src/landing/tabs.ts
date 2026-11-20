// Keyboard support for the [data-tabs] tablists of the landing pages (WAI-ARIA tabs pattern): arrows, Home and End
// move between tabs and select them; one tab is in the tab order. Clicks are handled by Base.astro, which switches
// the panels; this keeps the roving tabindex in step and reports each change.
export function enhanceTabs(root: ParentNode, onChange?: (panel: HTMLElement) => void): void {
	for (const list of root.querySelectorAll<HTMLElement>("[data-tabs] [role='tablist']")) {
		const tabs = [...list.querySelectorAll<HTMLButtonElement>("[role='tab']")];
		const sync = (): void => {
			for (const t of tabs) t.tabIndex = t.getAttribute("aria-selected") === "true" ? 0 : -1;
		};
		list.addEventListener("keydown", (e) => {
			const i = tabs.indexOf(document.activeElement as HTMLButtonElement);
			if (i < 0) return;
			let next = -1;
			if (e.key === "ArrowRight" || e.key === "ArrowDown") next = (i + 1) % tabs.length;
			else if (e.key === "ArrowLeft" || e.key === "ArrowUp") next = (i - 1 + tabs.length) % tabs.length;
			else if (e.key === "Home") next = 0;
			else if (e.key === "End") next = tabs.length - 1;
			if (next < 0) return;
			e.preventDefault();
			e.stopPropagation();
			tabs[next].focus();
			tabs[next].click();
		});
		list.addEventListener("click", () => {
			// Base.astro's handler runs on the document after this one: wait a task for its attributes.
			setTimeout(() => {
				sync();
				const selected = tabs.find((t) => t.getAttribute("aria-selected") === "true");
				const panel = selected ? document.getElementById(selected.getAttribute("aria-controls") ?? "") : null;
				if (panel && onChange) onChange(panel);
			});
		});
		sync();
	}
}
