import type * as React from "react";
import { describe, expect, test, vi } from "vitest";

import { tabbableElements, trapFocusTab } from "./focus-trap";

interface FakeOptions {
	tagName?: string;
	type?: string;
	name?: string;
	checked?: boolean;
	form?: HTMLElement | null;
	attrs?: Record<string, string>;
	tabIndex?: number;
	closest?: Record<string, unknown>;
	display?: string;
	visibility?: string;
	parent?: FakeEl | null;
}

class FakeEl {
	tagName: string;
	type: string;
	name: string;
	checked: boolean;
	form: HTMLElement | null;
	tabIndex: number;
	parentElement: FakeEl | null;
	ownerDocument: {
		activeElement: FakeEl | null;
		defaultView: {
			getComputedStyle: (el: FakeEl) => { display: string; visibility: string };
		};
	};
	children: FakeEl[] = [];
	focus = vi.fn(() => {
		this.ownerDocument.activeElement = this;
	});
	private attrs: Record<string, string>;
	private closestMap: Record<string, unknown>;
	private style: { display: string; visibility: string };
	order = 0;

	constructor(
		doc: FakeEl["ownerDocument"],
		opts: FakeOptions = {},
	) {
		this.ownerDocument = doc;
		this.tagName = (opts.tagName ?? "BUTTON").toUpperCase();
		this.type = opts.type ?? "";
		this.name = opts.name ?? "";
		this.checked = opts.checked ?? false;
		this.form = opts.form ?? null;
		this.attrs = { ...(opts.attrs ?? {}) };
		this.tabIndex = opts.tabIndex ?? 0;
		this.closestMap = opts.closest ?? {};
		this.style = {
			display: opts.display ?? "block",
			visibility: opts.visibility ?? "visible",
		};
		this.parentElement = opts.parent ?? null;
	}

	getComputedStyle(): { display: string; visibility: string } {
		return this.style;
	}

	hasAttribute(name: string): boolean {
		return Object.prototype.hasOwnProperty.call(this.attrs, name);
	}

	getAttribute(name: string): string | null {
		return this.hasAttribute(name) ? this.attrs[name] : null;
	}

	closest(selector: string): unknown {
		return this.closestMap[selector] ?? null;
	}

	querySelector(): null {
		return null;
	}

	querySelectorAll(): FakeEl[] {
		return this.children;
	}

	contains(other: FakeEl | null): boolean {
		if (!other) return false;
		if (other === this) return true;
		return this.children.includes(other);
	}

	compareDocumentPosition(other: FakeEl): number {
		if (this.order < other.order) return 4; // DOCUMENT_POSITION_FOLLOWING
		if (this.order > other.order) return 2; // DOCUMENT_POSITION_PRECEDING
		return 0;
	}
}

function makeDialog(items: Array<FakeOptions & { id?: string }>) {
	const doc: FakeEl["ownerDocument"] = {
		activeElement: null,
		defaultView: {
			getComputedStyle: (el: FakeEl) => el.getComputedStyle(),
		},
	};
	const dialog = new FakeEl(doc, { tagName: "DIV", tabIndex: -1 });
	dialog.order = 0;
	const elements = items.map((item, idx) => {
		const el = new FakeEl(doc, { ...item, parent: dialog });
		el.order = idx + 1;
		return el;
	});
	dialog.children = elements;
	return { doc, dialog, elements };
}

function tabEvent(
	dialog: FakeEl,
	opts: {
		shiftKey?: boolean;
		key?: string;
		defaultPrevented?: boolean;
		ctrlKey?: boolean;
	} = {},
) {
	let prevented = opts.defaultPrevented ?? false;
	const event = {
		key: opts.key ?? "Tab",
		shiftKey: opts.shiftKey ?? false,
		altKey: false,
		ctrlKey: opts.ctrlKey ?? false,
		metaKey: false,
		currentTarget: dialog as unknown as HTMLElement,
		get defaultPrevented() {
			return prevented;
		},
		preventDefault: vi.fn(() => {
			prevented = true;
		}),
	} as unknown as React.KeyboardEvent<HTMLElement>;
	return event;
}

describe("dialog focus trap (VV-443)", () => {
	test("wraps forward Tab from the last control back to the first", () => {
		const { doc, dialog, elements } = makeDialog([
			{ tagName: "INPUT" },
			{ tagName: "BUTTON" },
			{ tagName: "BUTTON" },
		]);
		doc.activeElement = elements[2];
		const ev = tabEvent(dialog);
		trapFocusTab(ev);
		expect(ev.preventDefault).toHaveBeenCalledTimes(1);
		expect(elements[0].focus).toHaveBeenCalledTimes(1);
		expect(doc.activeElement).toBe(elements[0]);
	});

	test("wraps Shift+Tab from the first control back to the last", () => {
		const { doc, dialog, elements } = makeDialog([
			{ tagName: "INPUT" },
			{ tagName: "BUTTON" },
			{ tagName: "BUTTON" },
		]);
		doc.activeElement = elements[0];
		const ev = tabEvent(dialog, { shiftKey: true });
		trapFocusTab(ev);
		expect(ev.preventDefault).toHaveBeenCalledTimes(1);
		expect(elements[2].focus).toHaveBeenCalledTimes(1);
		expect(doc.activeElement).toBe(elements[2]);
	});

	test("lets Tab move naturally between inner controls", () => {
		const { doc, dialog, elements } = makeDialog([
			{ tagName: "INPUT" },
			{ tagName: "BUTTON" },
			{ tagName: "BUTTON" },
		]);
		doc.activeElement = elements[1];
		const ev = tabEvent(dialog);
		trapFocusTab(ev);
		expect(ev.preventDefault).not.toHaveBeenCalled();
	});

	test("skips focus guards, disabled, hidden, inert and negative tabindex nodes", () => {
		const { dialog, elements } = makeDialog([
			{ tagName: "SPAN", attrs: { "data-base-ui-focus-guard": "" } },
			{ tagName: "BUTTON", attrs: { disabled: "" } },
			{ tagName: "BUTTON", attrs: { tabindex: "-1" }, tabIndex: -1 },
			{ tagName: "BUTTON", closest: { "[hidden]": true } },
			{ tagName: "BUTTON", closest: { "[inert]": true } },
			{ tagName: "BUTTON", display: "none" },
			{ tagName: "BUTTON", visibility: "hidden" },
			{ tagName: "INPUT" },
			{ tagName: "BUTTON" },
		]);
		const found = tabbableElements(dialog as unknown as HTMLElement);
		expect(found).toEqual([elements[7], elements[8]]);
	});

	test("collapses named radio groups to the checked radio or the first radio", () => {
		const { doc, dialog, elements } = makeDialog([
			{ tagName: "INPUT", type: "radio", name: "kind" },
			{ tagName: "INPUT", type: "radio", name: "kind", checked: true },
			{ tagName: "INPUT", type: "radio", name: "kind" },
			{ tagName: "BUTTON" },
		]);
		const found = tabbableElements(dialog as unknown as HTMLElement);
		expect(found).toEqual([elements[1], elements[3]]);

		// Shift+Tab from the checked radio (first tab stop) wraps to the last button.
		doc.activeElement = elements[1];
		const ev = tabEvent(dialog, { shiftKey: true });
		trapFocusTab(ev);
		expect(ev.preventDefault).toHaveBeenCalledTimes(1);
		expect(doc.activeElement).toBe(elements[3]);
	});

	test("does not hijack Tab when focus is in a nested portaled popup", () => {
		const { doc, dialog, elements } = makeDialog([
			{ tagName: "INPUT" },
			{ tagName: "BUTTON" },
		]);
		const nestedOption = new FakeEl(doc, { tagName: "DIV" });
		doc.activeElement = nestedOption;
		const ev = tabEvent(dialog);
		trapFocusTab(ev);
		expect(ev.preventDefault).not.toHaveBeenCalled();
		expect(elements[0].focus).not.toHaveBeenCalled();
	});

	test("keeps focus on the dialog container when it has no tabbable controls", () => {
		const { doc, dialog } = makeDialog([]);
		doc.activeElement = dialog;
		const ev = tabEvent(dialog);
		trapFocusTab(ev);
		expect(ev.preventDefault).toHaveBeenCalledTimes(1);
		expect(dialog.focus).toHaveBeenCalledTimes(1);
	});

	test("moves focus into controls when Tab or Shift+Tab is pressed on the dialog container", () => {
		const { doc, dialog, elements } = makeDialog([
			{ tagName: "INPUT" },
			{ tagName: "BUTTON" },
		]);
		doc.activeElement = dialog;
		trapFocusTab(tabEvent(dialog));
		expect(doc.activeElement).toBe(elements[0]);

		doc.activeElement = dialog;
		trapFocusTab(tabEvent(dialog, { shiftKey: true }));
		expect(doc.activeElement).toBe(elements[1]);
	});

	test("ignores already-prevented events and modified Tab shortcuts", () => {
		const { doc, dialog, elements } = makeDialog([{ tagName: "BUTTON" }]);
		doc.activeElement = elements[0];

		const prevented = tabEvent(dialog, { defaultPrevented: true });
		trapFocusTab(prevented);
		expect(elements[0].focus).not.toHaveBeenCalled();

		const ctrlTab = tabEvent(dialog, { ctrlKey: true });
		trapFocusTab(ctrlTab);
		expect(ctrlTab.preventDefault).not.toHaveBeenCalled();
	});
});
