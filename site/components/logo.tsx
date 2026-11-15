import { Link } from "@/components/link";
import { useT } from "@/lib/locale-context";

/** The Vukem Video mark: a play triangle above two timeline clips. */
export function LogoMark({ className = "size-6" }: { className?: string }) {
	return (
		<svg viewBox="0 0 32 32" fill="none" aria-hidden className={className}>
			<rect width="32" height="32" rx="8" fill="#7C5CFF" />
			<path d="M11 7.5v11l9-5.5z" fill="#fff" />
			<rect x="6" y="22" width="9" height="3" rx="1.5" fill="#fff" />
			<rect x="17" y="22" width="9" height="3" rx="1.5" fill="#fff" fillOpacity=".6" />
		</svg>
	);
}

export function Logo() {
	const t = useT();
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
