import type * as React from "react"

// Base UI 1.9.0 places focus-guard <span> sentinels around Dialog/AlertDialog
// popups and marks background siblings with aria-hidden + data-base-ui-inert
// instead of HTML inert, without looping Tab at the popup boundary. In
// Chromium, pressing Tab past the last control (or Shift+Tab before the first)
// moves through the guard to document.body and then into background page
// controls behind the modal (VV-443, WCAG 2.4.3). This module keeps Tab and
// Shift+Tab cycling inside an open modal popup without interfering with nested
// portaled popups (such as Select, Popover, Menu or a stacked Dialog).

const TABBABLE_SELECTOR = [
  "a[href]",
  "area[href]",
  "button:not([disabled])",
  'input:not([disabled]):not([type="hidden"])',
  "select:not([disabled])",
  "textarea:not([disabled])",
  "iframe",
  "audio[controls]",
  "video[controls]",
  '[contenteditable]:not([contenteditable="false"])',
  "details > summary:first-of-type",
  "[tabindex]",
].join(",")

const DOCUMENT_POSITION_FOLLOWING = 4

function explicitTabIndex(el: HTMLElement): number | null {
  const attr = el.getAttribute("tabindex")
  if (attr === null) return null
  const parsed = Number.parseInt(attr, 10)
  return Number.isNaN(parsed) ? null : parsed
}

function isDisabledInFieldset(el: HTMLElement): boolean {
  const fieldset = el.closest("fieldset[disabled]")
  if (!fieldset) return false
  const firstLegend = fieldset.querySelector(":scope > legend")
  return !firstLegend || !firstLegend.contains(el)
}

function isInClosedDetails(el: HTMLElement, container: HTMLElement): boolean {
  let node = el.parentElement
  while (node && node !== container) {
    if (node.tagName === "DETAILS" && !node.hasAttribute("open")) {
      const summary = node.querySelector(":scope > summary")
      if (!summary || !summary.contains(el)) return true
    }
    node = node.parentElement
  }
  return false
}

function isHiddenByStyle(el: HTMLElement, container: HTMLElement): boolean {
  const getStyle = el.ownerDocument?.defaultView?.getComputedStyle
  if (typeof getStyle !== "function") return false

  const selfStyle = getStyle(el)
  if (
    selfStyle.display === "none" ||
    selfStyle.visibility === "hidden" ||
    selfStyle.visibility === "collapse"
  ) {
    return true
  }

  let node = el.parentElement
  while (node && node !== container) {
    if (getStyle(node).display === "none") return true
    node = node.parentElement
  }
  return false
}

function isRadioInput(el: HTMLElement): el is HTMLInputElement {
  return el.tagName === "INPUT" && (el as HTMLInputElement).type === "radio"
}

function sameRadioGroup(a: HTMLElement, b: HTMLElement): boolean {
  if (!isRadioInput(a) || !isRadioInput(b)) return false
  return a.name !== "" && a.name === b.name && a.form === b.form
}

function isCandidateTabbable(el: HTMLElement, container: HTMLElement): boolean {
  if (el.hasAttribute("data-base-ui-focus-guard")) return false
  if (el.hasAttribute("disabled")) return false

  const explicit = explicitTabIndex(el)
  if (explicit !== null) {
    if (explicit < 0) return false
  } else if (el.tabIndex < 0) {
    return false
  }

  if (el.closest("[inert]")) return false
  if (el.closest("[hidden]")) return false
  if (isDisabledInFieldset(el)) return false
  if (isInClosedDetails(el, container)) return false
  if (isHiddenByStyle(el, container)) return false
  return true
}

// Returns the elements inside container that participate in sequential keyboard
// navigation (Tab order), with radio groups collapsed to their single tab stop
// (the checked radio if any, otherwise the first radio in the group).
export function tabbableElements(container: HTMLElement): HTMLElement[] {
  const raw = Array.from(
    container.querySelectorAll<HTMLElement>(TABBABLE_SELECTOR),
  ).filter((el) => isCandidateTabbable(el, container))

  // Collapse each named radio group to its single tab stop.
  const radioStops = new Map<HTMLElement | null, Map<string, HTMLInputElement>>()
  for (const el of raw) {
    if (!isRadioInput(el) || el.name === "") continue
    const scope = el.form ?? container
    let byName = radioStops.get(scope)
    if (!byName) {
      byName = new Map()
      radioStops.set(scope, byName)
    }
    const existing = byName.get(el.name)
    if (!existing || (el.checked && !existing.checked)) {
      byName.set(el.name, el)
    }
  }

  const filtered = raw.filter((el) => {
    if (!isRadioInput(el) || el.name === "") return true
    const scope = el.form ?? container
    return radioStops.get(scope)?.get(el.name) === el
  })

  const positive: Array<{ el: HTMLElement; tabIndex: number; order: number }> = []
  const natural: HTMLElement[] = []
  for (let i = 0; i < filtered.length; i++) {
    const el = filtered[i]
    const ti = explicitTabIndex(el) ?? el.tabIndex
    if (ti > 0) {
      positive.push({ el, tabIndex: ti, order: i })
    } else {
      natural.push(el)
    }
  }
  positive.sort((a, b) => a.tabIndex - b.tabIndex || a.order - b.order)
  return [...positive.map((entry) => entry.el), ...natural]
}

export function trapFocusTab(event: React.KeyboardEvent<HTMLElement>): void {
  if (
    event.key !== "Tab" ||
    event.defaultPrevented ||
    event.altKey ||
    event.ctrlKey ||
    event.metaKey
  ) {
    return
  }

  const container = event.currentTarget
  const active = container.ownerDocument?.activeElement as HTMLElement | null
  // Do not hijack Tab from a nested portaled popup (Select, Menu, Popover or
  // stacked Dialog) whose DOM lives outside this popup container.
  if (!active || !container.contains(active)) {
    return
  }

  const tabbables = tabbableElements(container)
  if (tabbables.length === 0) {
    event.preventDefault()
    container.focus()
    return
  }

  const first = tabbables[0]
  const last = tabbables[tabbables.length - 1]

  if (active === container) {
    event.preventDefault()
    ;(event.shiftKey ? last : first).focus()
    return
  }

  const firstTabIndex = explicitTabIndex(first) ?? first.tabIndex
  const lastTabIndex = explicitTabIndex(last) ?? last.tabIndex
  const naturalOrder = firstTabIndex === 0 && lastTabIndex === 0

  if (event.shiftKey) {
    if (
      active === first ||
      sameRadioGroup(active, first) ||
      (naturalOrder &&
        Boolean(
          active.compareDocumentPosition(first) & DOCUMENT_POSITION_FOLLOWING,
        ))
    ) {
      event.preventDefault()
      last.focus()
    }
    return
  }

  if (
    active === last ||
    sameRadioGroup(active, last) ||
    (naturalOrder &&
      Boolean(
        last.compareDocumentPosition(active) & DOCUMENT_POSITION_FOLLOWING,
      ))
  ) {
    event.preventDefault()
    first.focus()
  }
}
