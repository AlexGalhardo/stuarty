import type { ReactNode } from "react";

// Names and text that come from agents or files may carry bidirectional
// control characters (SDD-007 section 8.5 item 3). Isolating them keeps such
// text from reordering the interface around it (SDD-012 section 1.2).
export default function UntrustedText({
	children,
	className = "",
	"aria-hidden": ariaHidden,
}: {
	children: ReactNode;
	className?: string;
	"aria-hidden"?: "true";
}) {
	return (
		<span
			dir="auto"
			aria-hidden={ariaHidden}
			className={`[unicode-bidi:isolate] ${className}`}
		>
			{children}
		</span>
	);
}
