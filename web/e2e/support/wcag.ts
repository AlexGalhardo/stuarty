import type { Page } from "@playwright/test";

// Page-side measurements for the WCAG 2.2 criteria that a rule engine such as
// axe does not decide (SDD-012 section 18.6). Each function runs in the page and
// returns plain findings, so a spec reports every one at once.

export interface Undersized {
	target: string;
	size: string;
}

// WCAG 2.5.8 Target Size (Minimum). A pointer target is at least 24 by 24 CSS
// pixels, unless: it is inline in a sentence, it is disabled, a 24 px circle
// centred on it touches no other target (the spacing exception), or the same
// function is offered by another control (`equivalents`, a selector list with
// its reason in the spec). Elements for assistive technology only (sr-only,
// 1 px clipped) are not targets.
export async function undersizedTargets(
	page: Page,
	equivalents: string[] = [],
): Promise<Undersized[]> {
	return page.evaluate((equivalent) => {
		const MIN = 24;
		const query =
			"a[href], button, input:not([type=hidden]), select, textarea, summary, [role=button], [role=link], [role=checkbox], [role=radio], [role=switch], [role=tab], [role=menuitem], [role=slider], [role=option], [role=combobox], [role=gridcell][tabindex]";
		const named = (el: Element): string => {
			const label =
				el.getAttribute("aria-label") ??
				(el.textContent ?? "").trim().slice(0, 30);
			return `${el.tagName.toLowerCase()}${
				el.getAttribute("role") ? `[role=${el.getAttribute("role")}]` : ""
			} "${label}"`;
		};
		const visible = (el: Element): DOMRect | null => {
			const style = getComputedStyle(el);
			if (style.display === "none" || style.visibility === "hidden")
				return null;
			if (style.pointerEvents === "none") return null;
			const rect = el.getBoundingClientRect();
			if (rect.width <= 1 || rect.height <= 1) return null;
			for (let p: Element | null = el; p; p = p.parentElement) {
				if (getComputedStyle(p).display === "none") return null;
			}
			return rect;
		};
		const all = Array.from(document.querySelectorAll(query))
			.map((el) => ({ el, rect: visible(el) }))
			.filter((x): x is { el: Element; rect: DOMRect } => x.rect !== null)
			.filter(
				({ el }) =>
					!(el as HTMLButtonElement).disabled &&
					el.getAttribute("aria-disabled") !== "true",
			);
		// A radio or checkbox drawn by a custom control keeps its native input
		// visually hidden over the control: the control is the target.
		const targets = all.filter(({ el, rect }) => {
			const style = getComputedStyle(el);
			if (style.opacity === "0") return false;
			if (style.display === "inline" && el.tagName === "A") {
				const parent = el.parentElement;
				if (
					parent &&
					(parent.textContent ?? "").trim() !== (el.textContent ?? "").trim()
				)
					return false;
			}
			void rect;
			return true;
		});
		const found: Undersized[] = [];
		for (const { el, rect } of targets) {
			if (rect.width >= MIN - 0.5 && rect.height >= MIN - 0.5) continue;
			if (equivalent.some((selector) => el.matches(selector))) continue;
			const cx = rect.left + rect.width / 2;
			const cy = rect.top + rect.height / 2;
			const clash = targets.some(({ el: other, rect: r }) => {
				if (other === el || el.contains(other) || other.contains(el))
					return false;
				// Closest point of the other target to the centre of this one.
				const nx = Math.max(r.left, Math.min(cx, r.right));
				const ny = Math.max(r.top, Math.min(cy, r.bottom));
				const small = r.width < MIN || r.height < MIN;
				// Two undersized targets must keep their 24 px circles apart; a
				// full-size one only has to stay out of this circle.
				const radius = small ? MIN : MIN / 2;
				const ocx = r.left + r.width / 2;
				const ocy = r.top + r.height / 2;
				return small
					? Math.hypot(ocx - cx, ocy - cy) < MIN
					: Math.hypot(nx - cx, ny - cy) < radius;
			});
			if (!clash) continue;
			found.push({
				target: named(el),
				size: `${Math.round(rect.width)}x${Math.round(rect.height)}`,
			});
		}
		return found;
	}, equivalents);
}

// WCAG 1.4.10 Reflow: at 320 CSS px the page scrolls in one direction only.
export async function horizontalOverflow(page: Page): Promise<string | null> {
	return page.evaluate(() => {
		const root = document.documentElement;
		if (root.scrollWidth <= root.clientWidth + 1) return null;
		const wide = Array.from(document.body.querySelectorAll("*"))
			.filter((el) => el.getBoundingClientRect().right > root.clientWidth + 1)
			.slice(0, 5)
			.map(
				(el) =>
					`${el.tagName.toLowerCase()}.${String(el.getAttribute("class") ?? "").slice(0, 40)} ${Math.round(el.getBoundingClientRect().right)}`,
			);
		return `scrollWidth ${root.scrollWidth} > clientWidth ${root.clientWidth}: ${wide.join(" | ")}`;
	});
}

// WCAG 1.4.12 Text Spacing: the author-set text spacing of the criterion is
// applied through a user style sheet, and no text may be clipped or overlap.
// A clipped element is one that hides overflow while its content is larger.
export const TEXT_SPACING_CSS = `
* {
	line-height: 1.5 !important;
	letter-spacing: 0.12em !important;
	word-spacing: 0.16em !important;
}
p { margin-bottom: 2em !important; }
`;

export async function clippedText(page: Page): Promise<string[]> {
	return page.evaluate(() => {
		const out: string[] = [];
		for (const el of Array.from(document.body.querySelectorAll("*"))) {
			if ((el.textContent ?? "").trim() === "") continue;
			if (el.children.length > 0 && el.tagName !== "BUTTON") continue;
			const style = getComputedStyle(el);
			if (style.display === "none" || style.visibility === "hidden") continue;
			if (el.closest(".sr-only, [aria-hidden=true]")) continue;
			const clipsY = ["hidden", "clip"].includes(style.overflowY);
			const clipsX = ["hidden", "clip"].includes(style.overflowX);
			const truncates = style.textOverflow === "ellipsis";
			if (truncates) continue;
			const h = el as HTMLElement;
			if (
				(clipsY && h.scrollHeight > h.clientHeight + 1) ||
				(clipsX && h.scrollWidth > h.clientWidth + 1)
			) {
				out.push(
					`${el.tagName.toLowerCase()} "${(el.textContent ?? "").trim().slice(0, 30)}"`,
				);
			}
		}
		return out;
	});
}

// WCAG 2.4.11 Focus Not Obscured (Minimum): the focused element is not entirely
// hidden by author content. Reports the focused element when its centre and all
// four corners (inset by 1 px) resolve to something outside it.
export async function obscuredFocus(page: Page): Promise<string | null> {
	return page.evaluate(async () => {
		// WebKit scrolls a focused element into view after the key event, so the
		// position is read once two frames have drawn.
		await new Promise<void>((done) =>
			requestAnimationFrame(() => requestAnimationFrame(() => done())),
		);
		const el = document.activeElement;
		if (!el || el === document.body) return null;
		const rect = el.getBoundingClientRect();
		if (rect.width === 0 || rect.height === 0) return null;
		const points: [number, number][] = [
			[rect.left + rect.width / 2, rect.top + rect.height / 2],
			[rect.left + 1, rect.top + 1],
			[rect.right - 1, rect.top + 1],
			[rect.left + 1, rect.bottom - 1],
			[rect.right - 1, rect.bottom - 1],
		];
		const inside = (x: number, y: number): boolean => {
			if (x < 0 || y < 0 || x > innerWidth || y > innerHeight) return false;
			const top = document.elementFromPoint(x, y);
			return top !== null && (el.contains(top) || top.contains(el));
		};
		if (points.some(([x, y]) => inside(x, y))) return null;
		return `${el.tagName.toLowerCase()} "${(el.getAttribute("aria-label") ?? el.textContent ?? "").trim().slice(0, 30)}"`;
	});
}
