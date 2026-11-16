import { createInertiaApp } from "@inertiajs/react";

import AppLayout from "./layouts/AppLayout";
import { installStyleNonce } from "./lib/cspNonce";
import { installLangHeader, reconcileLang } from "./lib/lang";

// Dark by default (D-024, SDD-012 section 17): the "dark" class on <html> is
// set before paint by the inline script of app.html, from localStorage.theme
// ("light" turns it off). The system preference is deliberately ignored. This
// repeats the rule for the case where the script did not run.
try {
	document.documentElement.classList.toggle(
		"dark",
		localStorage.getItem("theme") !== "light",
	);
} catch {
	document.documentElement.classList.add("dark");
}

// The language of this browser rides on every request (SDD-012 section 16.3).
installLangHeader();

// The scroll lock of a Radix select or dialog writes a <style> element.
installStyleNonce();

// The @inertiajs/vite plugin resolves pages from ./pages and mounts the app.
// The nonce lets the progress bar styles through the CSP.
// It mounts on #app, the default inertia.Config.RootID; when you change
// RootID, pass the same value here as `id`.
createInertiaApp({
	layout: () => AppLayout,
	nonce: document.querySelector<HTMLMetaElement>('meta[property="csp-nonce"]')
		?.nonce,
	progress: { color: "oklch(0.62 0.17 255)" },
}).finally(() => {
	reconcileLang();
	// app.html sets data-hydrating; style it (button:disabled, a pending hint)
	// for the gap before the app mounts.
	document.documentElement.removeAttribute("data-hydrating");
});
