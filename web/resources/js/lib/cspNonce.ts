// The page carries the nonce of its CSP on a csp-nonce meta element (SDD-012
// section 2.2). Undefined where there is no document or no such element.
export function pageNonce(): string | undefined {
	if (typeof document === "undefined") return undefined;
	return (
		document.querySelector<HTMLMetaElement>('meta[property="csp-nonce"]')
			?.nonce || undefined
	);
}

// react-style-singleton (behind the scroll lock of every Radix modal) writes a
// <style> element and takes its nonce from the global __webpack_nonce__. Without
// it the element breaks style-src 'nonce-...' the first time a dialog opens.
export function installStyleNonce(): void {
	const nonce = pageNonce();
	if (nonce) {
		(globalThis as { __webpack_nonce__?: string }).__webpack_nonce__ = nonce;
	}
}
