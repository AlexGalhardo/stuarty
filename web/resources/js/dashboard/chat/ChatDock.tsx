import { Settings2, X } from "lucide-react";
import { type ReactNode, useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { useShell } from "../../shell/ShellContext";
import ChatConversation from "./ChatConversation";
import ProfileSetup from "./ProfileSetup";

const NO_PROFILES = {
	profiles: [],
	suggestions: [],
	can_write: false,
	available: false,
};

// The chat panel of the right dock (SDD-017, RF-313): the Chat button of the top bar opens the
// conversation with the agent the user hosts. The project it talks about is the one in the address
// when there is one, else the one the person picked last (the shell reads it, so the scene bar and
// the panel show the same session). It sends the person's words to the server and draws what
// comes back; everything the agent writes is text.
export default function ChatDock(): ReactNode {
	const t = useT();
	const {
		chat,
		chatOpen: open,
		setChatOpen: setOpen,
		chatButton: toggle,
	} = useShell();
	const [managing, setManaging] = useState(false);
	const composer = useRef<HTMLTextAreaElement | null>(null);
	const heading = useRef<HTMLHeadingElement | null>(null);
	const closed = useRef(false);
	const ready = chat.profiles !== null;
	const connected = (chat.profiles?.profiles.length ?? 0) > 0;
	const showSetup = ready && (!connected || managing);

	// The panel takes the focus when it opens (the button that had it is gone), the composer takes it when
	// the conversation is ready, and the button gets it back on close
	useEffect(() => {
		if (open) heading.current?.focus();
	}, [open]);
	useEffect(() => {
		if (open && connected && !managing) composer.current?.focus();
		if (!open && closed.current) {
			closed.current = false;
			toggle.current?.focus();
		}
	}, [open, connected, managing, toggle]);

	const close = (): void => {
		closed.current = true;
		setManaging(false);
		setOpen(false);
	};

	// Closed, the panel is nothing: the Chat button of the top bar opens it (SDD-012 section 23.4).
	if (!open) return null;

	return (
		<aside
			id="chat-panel"
			aria-label={t("ui.chat.title")}
			data-testid="chat-panel"
			onKeyDown={(event) => {
				if (event.key === "Escape") close();
			}}
			className="flex h-dvh w-full flex-col gap-3 border-s bg-background p-3 z-50 max-md:fixed max-md:inset-0 md:sticky md:top-0 md:w-96"
		>
			<header className="flex items-center justify-between gap-2">
				<h2 ref={heading} tabIndex={-1} className="text-base font-semibold">
					{t("ui.chat.title")}
				</h2>
				<div className="flex items-center gap-1">
					{ready && connected && (
						<Button
							type="button"
							size="icon-sm"
							variant="ghost"
							aria-pressed={managing}
							aria-label={t("ui.chat.manage")}
							onClick={() => setManaging((was) => !was)}
							data-testid="chat-manage"
						>
							<Settings2 aria-hidden="true" />
						</Button>
					)}
					<Button
						type="button"
						size="icon-sm"
						variant="ghost"
						aria-label={t("ui.chat.close")}
						onClick={close}
						data-testid="chat-close"
					>
						<X aria-hidden="true" />
					</Button>
				</div>
			</header>
			{!ready ? (
				<p role="status" className="text-sm text-muted-foreground">
					{t("ui.chat.loading")}
				</p>
			) : showSetup ? (
				<section
					aria-label={t("ui.chat.manage")}
					// biome-ignore lint/a11y/noNoninteractiveTabindex: a scrollable region must be reachable by keyboard
					tabIndex={0}
					className="min-h-0 flex-1 overflow-y-auto focus-visible:ring-3 focus-visible:ring-ring/50"
				>
					<ProfileSetup
						profiles={chat.profiles ?? NO_PROFILES}
						busy={chat.busy}
						failure={chat.failure}
						onSave={chat.saveProfile}
						onRemove={(name) => void chat.removeProfile(name)}
					/>
				</section>
			) : (
				<ChatConversation chat={chat} composerRef={composer} />
			)}
		</aside>
	);
}
