import { type ReactNode, useCallback, useSyncExternalStore } from "react";

import type { Failure } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { ContextMenuItem } from "@/components/ui/context-menu";
import {
	canRevealAsset,
	getDesktopBridge,
	revealAsset,
	subscribeDesktopBridge,
} from "@/lib/desktopBridge";
import { useT } from "@/lib/i18n";
import { useSession } from "@/lib/session";
import { createClient } from "../editor/api/client";
import { ApiError } from "../editor/api/errors";
import { HttpTransport } from "../editor/api/http";

// What a person may do with the folder of a stored file (SDD-011 section 14.10, SDD-012 section 23.5 item 5):
// the desktop app opens the file manager ("show"), the browser in local mode copies the path ("copy"), and in
// network mode neither is offered (`kind` is null). The server decides who may and refuses anyone else; this
// only draws what it would accept. The page sends the hash, never a path, and the path it gets goes to the
// shell or to the clipboard: it is drawn nowhere except as `display`, isolated, in the notice of a copy.
export interface Reveal {
	kind: "show" | "copy" | null;
	// Local mode with a session that is not a person's: drawn, but off, with the reason beside it.
	disabled: boolean;
	run(hash: string): Promise<RevealOutcome>;
}

export type RevealOutcome = { failure: Failure } | { notice: RevealNotice };

export type RevealNotice =
	| { kind: "shown" }
	| { kind: "copied"; display: string };

function failureOfAny(error: unknown): Failure {
	if (error instanceof ApiError) {
		return {
			code: error.code,
			message: error.message,
			hint: error.hint,
			path: error.path,
		};
	}
	return {
		code: "E_DESKTOP",
		message: error instanceof Error ? error.message : String(error),
	};
}

// Which action the page offers: nothing in network mode, where the server never names a path on its disk; the
// shell's own action when the page runs in the desktop app; copying the path in the browser in local mode.
export function revealKindOf(
	mode: "local" | "network",
	bridgeCanReveal: boolean,
): Reveal["kind"] {
	if (mode === "network") return null;
	return bridgeCanReveal ? "show" : "copy";
}

export function useReveal(): Reveal {
	const { privileged, mode } = useSession();
	const bridge = useSyncExternalStore(
		subscribeDesktopBridge,
		getDesktopBridge,
		() => null,
	);
	const kind = revealKindOf(mode, canRevealAsset(bridge));
	const run = useCallback(
		async (hash: string): Promise<RevealOutcome> => {
			try {
				const result = await createClient(
					new HttpTransport("/api/v1"),
				).revealAsset(hash);
				if (kind === "show") {
					await revealAsset(bridge, result.path);
					return { notice: { kind: "shown" } };
				}
				await navigator.clipboard.writeText(result.path);
				return { notice: { kind: "copied", display: result.display } };
			} catch (error) {
				return { failure: failureOfAny(error) };
			}
		},
		[bridge, kind],
	);
	return { kind, disabled: kind !== null && !privileged, run };
}

// The polite sentence after a reveal. The path is drawn isolated: it may carry bidirectional characters.
export function RevealNoticeText({
	notice,
}: {
	notice: RevealNotice | null;
}): ReactNode {
	const t = useT();
	if (notice === null) return null;
	return (
		<p role="status" data-testid="reveal-notice" className="text-sm text-text">
			{notice.kind === "shown" ? (
				t("ui.assets.reveal.shown")
			) : (
				<>
					{t("ui.assets.reveal.copied")}{" "}
					<UntrustedText className="font-mono break-all">
						{notice.display}
					</UntrustedText>
				</>
			)}
		</p>
	);
}

// The menu item of a card or a row: nothing in network mode, off with the reason in the detail when the session
// is not a person's.
export function RevealMenuItem({
	reveal,
	onSelect,
}: {
	reveal: Pick<Reveal, "kind" | "disabled">;
	onSelect(): void;
}): ReactNode {
	const t = useT();
	if (reveal.kind === null) return null;
	return (
		<ContextMenuItem disabled={reveal.disabled} onSelect={onSelect}>
			{t(
				reveal.kind === "show"
					? "ui.assets.reveal.show"
					: "ui.assets.reveal.copy",
			)}
		</ContextMenuItem>
	);
}
