import { type ReactNode, useEffect, useState } from "react";

import { cn } from "@/lib/cn";
import { createClient } from "../editor/api/client";
import { HttpTransport } from "../editor/api/http";

// The release the running server reports (GET /system). A leading v is kept,
// so 0.28.0 and v0.28.0 both read as the tag.
export function releaseLabel(version: string): string {
	const bare = version.trim();
	if (bare === "") return "";
	return bare.startsWith("v") ? bare : `v${bare}`;
}

// Fixed at the bottom right of a page that has no sidebar (the sign-in), so a build is identifiable without
// opening Settings; in the footer of the sidebar (epic 75) it is a line of the footer, next to the mode.
export function ReleaseMarkView({
	version,
	inline = false,
	compact = false,
}: {
	version: string;
	inline?: boolean;
	// In the collapsed rail, let long release labels wrap within the available width (VV-437).
	compact?: boolean;
}): ReactNode {
	const label = releaseLabel(version);
	if (label === "") return null;
	if (inline) {
		return (
			<span
				data-testid="release-mark"
				className={cn(
					"text-xs text-muted-foreground",
					compact
						? "max-w-10 wrap-anywhere text-center font-sans tracking-tight"
						: "font-mono",
				)}
			>
				{label}
			</span>
		);
	}
	return (
		<p
			data-testid="release-mark"
			className="pointer-events-none fixed right-[max(1rem,env(safe-area-inset-right,0px))] bottom-[max(0.25rem,env(safe-area-inset-bottom,0px))] z-40 rounded bg-background/80 px-1.5 py-0.5 font-mono text-xs text-muted-foreground"
		>
			{label}
		</p>
	);
}

export default function ReleaseMark({
	inline = false,
	compact = false,
	known,
}: {
	inline?: boolean;
	compact?: boolean;
	// The release the page already has (a shared prop): no request is made. A page with no session must pass it,
	// because GET /system answers it 401 and leaves an error in the console (VV-444).
	known?: string;
}): ReactNode {
	const [version, setVersion] = useState("");
	useEffect(() => {
		if (known !== undefined) return;
		let live = true;
		createClient(new HttpTransport("/api/v1"))
			.getSystem()
			.then((result) => {
				if (live) setVersion(result.version);
			})
			.catch(() => undefined);
		return () => {
			live = false;
		};
	}, [known]);
	return (
		<ReleaseMarkView
			version={known ?? version}
			inline={inline}
			compact={compact}
		/>
	);
}
