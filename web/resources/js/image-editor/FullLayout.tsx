import { usePage } from "@inertiajs/react";
import { type ReactNode, useEffect } from "react";

import { TooltipProvider } from "@/components/ui/tooltip";
import ChatDock from "@/dashboard/chat/ChatDock";
import { useChat } from "@/dashboard/chat/useChat";
import { type I18nProps, I18nProvider } from "@/lib/i18n";
import { useLockWatch } from "@/lib/lockWatch";
import { SessionProvider } from "@/lib/session";
import type { SharedProps } from "@/types";
import ConnectedSceneBar from "../shell/ConnectedSceneBar";
import { projectInUrl, ShellProvider } from "../shell/ShellContext";
import { useInbox } from "../shell/useInbox";
import { usePageVisible } from "../shell/usePageVisible";

// The frame of the image editor: the whole window, with no sidebar, as the final design draws it and as the
// video editor is (the top bar and the panels are the editor's own, the scene bar of the agent stays at the
// foot). It replaces the dashboard frame for this page only (Show.layout), so it carries the providers
// AppLayout gives every other page, and the chat the scene bar and the Chat button talk to.
export default function FullLayout({
	children,
}: {
	children: ReactNode;
}): ReactNode {
	const page = usePage<SharedProps & I18nProps>();
	const { messages = {}, locale = "en", locales } = page.props;
	useEffect(() => {
		document.documentElement.lang = locale;
	}, [locale]);
	useLockWatch(page.props.account !== undefined);
	const visible = usePageVisible();
	const chat = useChat(visible, projectInUrl(page.url));
	const inbox = useInbox(visible);

	return (
		<I18nProvider messages={messages} locale={locale} locales={locales}>
			<SessionProvider session={page.props.session}>
				<TooltipProvider>
					<ShellProvider chat={chat} inbox={inbox}>
						<div className="flex h-dvh min-h-[34rem] bg-background">
							<div className="flex min-w-0 flex-1 flex-col">
								<main
									id="main"
									tabIndex={-1}
									className="flex min-h-0 flex-1 flex-col outline-none"
								>
									{children}
								</main>
								<ConnectedSceneBar />
							</div>
							<div data-testid="right-dock" data-dock="right">
								<ChatDock />
							</div>
						</div>
					</ShellProvider>
				</TooltipProvider>
			</SessionProvider>
		</I18nProvider>
	);
}
