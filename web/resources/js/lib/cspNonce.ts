// react-style-singleton (behind the scroll lock of every Radix modal) writes a
// <style> element and takes its nonce from the global __webpack_nonce__. Without
// it the element breaks style-src 'nonce-...' the first time a dialog opens.
// The page carries the nonce on its csp-nonce meta element.
export function installStyleNonce(): void {
	const nonce = document.querySelector<HTMLMetaElement>(
		'meta[property="csp-nonce"]',
	)?.nonce;
	if (nonce) {
		(globalThis as { __webpack_nonce__?: string }).__webpack_nonce__ = nonce;
	}
}
