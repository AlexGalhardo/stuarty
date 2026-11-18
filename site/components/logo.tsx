import { Link } from "@/components/link";
import { ui as t } from "@/lib/ui-text";

/** The Vukem Video mark: the corners of a viewfinder around a play triangle. */
export function LogoMark({ className = "size-6" }: { className?: string }) {
	return (
		<svg viewBox="0 0 32 32" fill="none" aria-hidden className={className}>
			<path
				d="M4 10V7a3 3 0 0 1 3-3h3M22 4h3a3 3 0 0 1 3 3v3M28 22v3a3 3 0 0 1-3 3h-3M10 28H7a3 3 0 0 1-3-3v-3"
				stroke="#7C5CFF"
				strokeWidth="2.4"
				strokeLinecap="round"
				strokeLinejoin="round"
			/>
			<path d="M13 10.5v11l9-5.5z" fill="#7C5CFF" />
		</svg>
	);
}

export function Logo() {
	return (
		<Link
			href="/"
			className="flex items-center gap-2 rounded-md font-semibold tracking-tight text-fg"
			aria-label={t.logoLabel}
		>
			<LogoMark />
			<span className="text-[0.9375rem]">Vukem Video</span>
		</Link>
	);
}
