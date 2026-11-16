import { ChevronDown, ChevronRight } from "lucide-react";
import { type ReactNode, useId, useState } from "react";

// A section of the inspector (docs/design/alpha-editor.md section 3.1): a
// header button with aria-expanded and the fields under it.
export default function Section({
	title,
	name,
	defaultOpen = true,
	children,
}: {
	title: string;
	// Names the section for the keys that open it (`data-section`).
	name?: string;
	defaultOpen?: boolean;
	children: ReactNode;
}): ReactNode {
	const [open, setOpen] = useState(defaultOpen);
	const bodyId = useId();
	const Chevron = open ? ChevronDown : ChevronRight;
	return (
		<section data-section={name} className="flex flex-col gap-2">
			<button
				type="button"
				aria-expanded={open}
				aria-controls={bodyId}
				className="flex h-8 items-center gap-1 text-left text-sm font-semibold"
				onClick={() => setOpen((was) => !was)}
			>
				<Chevron aria-hidden="true" className="size-4" />
				{title}
			</button>
			<div id={bodyId} hidden={!open} className="flex flex-col gap-2 pl-1">
				{children}
			</div>
		</section>
	);
}
