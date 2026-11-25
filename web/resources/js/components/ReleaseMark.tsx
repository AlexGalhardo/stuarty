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
	// In the collapsed rail of the sidebar, which is 40 px wide inside its padding: a smaller type, so the tag
	// is not cut at the edge of the window (VV-437).
	compact?: boolean;
}): ReactNode {
	const label = releaseLabel(version);
	if (label === "") return null;
	if (inline) {
		return (
			<span
				data-testid="release-mark"
				className={cn(
					"font-mono text-xs text-muted-foreground",
					compact && "text-[10px] tracking-tighter",
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
}: {
	inline?: boolean;
	compact?: boolean;
}): ReactNode {
	const [version, setVersion] = useState("");
	useEffect(() => {
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
	}, []);
	return (
		<ReleaseMarkView version={version} inline={inline} compact={compact} />
	);
}
