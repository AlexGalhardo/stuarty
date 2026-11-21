// The product pictures of the home open enlarged in a native modal <dialog>: a click, or Enter or Space on the
// focused picture. The dialog shows the theme that is visible at that moment (the light and dark pair crossfade),
// closes on Esc, on the backdrop and on its button, keeps the focus inside (showModal makes the rest inert) and gives
// it back to the picture. Without JavaScript the pictures stay plain images.

export function mountZoom(): void {
	const dialog = document.querySelector<HTMLDialogElement>("[data-zoom-dialog]");
	const stage = dialog?.querySelector<HTMLElement>("[data-zoom-stage]");
	if (!dialog || !stage) return;
	let opener: HTMLElement | null = null;

	// The picture of a frame that is on screen now: the dark one while it is faded in, else the light one.
	const visiblePicture = (frame: HTMLElement): HTMLPictureElement | null => {
		const dark = frame.querySelector<HTMLPictureElement>("picture.shot-dark");
		if (dark && Number(getComputedStyle(dark).opacity) >= 0.5) return dark;
		return frame.querySelector<HTMLPictureElement>("picture:not(.shot-dark)") ?? frame.querySelector("picture");
	};
	const altOf = (frame: HTMLElement): string =>
		[...frame.querySelectorAll("img")].map((i) => i.alt).find((a) => a) ?? "Screenshot of Stuarty";

	const open = (frame: HTMLElement): void => {
		const picture = visiblePicture(frame);
		if (!picture) return;
		const copy = picture.cloneNode(true) as HTMLPictureElement;
		copy.removeAttribute("class");
		for (const el of copy.querySelectorAll<HTMLSourceElement | HTMLImageElement>("source, img")) el.sizes = "94vw";
		const img = copy.querySelector("img");
		if (img) {
			img.alt = altOf(frame);
			img.loading = "eager";
			img.removeAttribute("width");
			img.removeAttribute("height");
		}
		stage.replaceChildren(copy);
		opener = frame;
		dialog.showModal();
	};

	for (const frame of document.querySelectorAll<HTMLElement>('main[data-page="viewport"] .showcase')) {
		if (!frame.querySelector("picture")) continue;
		frame.setAttribute("role", "button");
		frame.tabIndex = 0;
		frame.setAttribute("aria-haspopup", "dialog");
		frame.setAttribute("aria-label", `Enlarge: ${altOf(frame)}`);
		frame.dataset.zoomable = "";
		frame.addEventListener("click", () => open(frame));
		frame.addEventListener("keydown", (e) => {
			if (e.key !== "Enter" && e.key !== " ") return;
			e.preventDefault();
			open(frame);
		});
	}

	dialog.querySelector("[data-zoom-close]")?.addEventListener("click", () => dialog.close());
	// A click on the backdrop lands on the dialog itself; a click on the picture lands on the image.
	dialog.addEventListener("click", (e) => {
		if (e.target === dialog) dialog.close();
	});
	// The close button is the one control inside, so Tab and Shift+Tab stay on it instead of leaving the page.
	dialog.addEventListener("keydown", (e) => {
		if (e.key !== "Tab") return;
		e.preventDefault();
		dialog.querySelector<HTMLElement>("[data-zoom-close]")?.focus();
	});
	dialog.addEventListener("close", () => {
		stage.replaceChildren();
		opener?.focus();
		opener = null;
	});
}
