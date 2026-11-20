import { createContext, type ReactNode, useContext } from "react";

import { useT } from "@/lib/i18n";

// What the server says about the session of this page (the shared prop `session`): whether it may do what only a
// person does (approve, set a price, keep a note, mark a skill as read) and in which mode the server runs. The
// server decides and refuses the call anyway; the page only draws those controls disabled, with the reason and
// where to do it, instead of letting the person click and read a refusal (VV-319).
export interface SessionInfo {
	privileged: boolean;
	mode: "local" | "network";
}

// Until the server says so, nothing is offered that it would refuse.
const NONE: SessionInfo = { privileged: false, mode: "local" };

const SessionContext = createContext<SessionInfo>(NONE);

export function SessionProvider({
	session,
	children,
}: {
	session: SessionInfo | undefined;
	children: ReactNode;
}): ReactNode {
	return (
		<SessionContext.Provider value={session ?? NONE}>
			{children}
		</SessionContext.Provider>
	);
}

export function useSession(): SessionInfo {
	return useContext(SessionContext);
}

// The short visible reason, for a session that is not a person's; nothing for one that is. The control it
// explains is described by it (aria-describedby).
export function PersonOnlyNote({ id }: { id: string }): ReactNode {
	const t = useT();
	const { privileged, mode } = useSession();
	if (privileged) return null;
	return (
		<p id={id} className="text-xs text-muted-foreground">
			{t(`ui.session.person_only.${mode}`)}
		</p>
	);
}
