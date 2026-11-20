import { Link } from "@/components/link";
import { withBase } from "@/lib/site";
import { ui as t } from "@/lib/ui-text";

/** The Stuarty mark: the official icon, scaled from the source by internal/tools/installericon. */
export function LogoMark({ className = "size-6" }: { className?: string }) {
	return <img src={withBase("/logo.png")} alt="" width={24} height={24} className={className} />;
}

export function Logo() {
	return (
		<Link
			href="/"
			className="flex items-center gap-2 rounded-md font-semibold tracking-tight text-fg"
			aria-label={t.logoLabel}
		>
			<LogoMark />
			<span className="text-[0.9375rem]">Stuarty</span>
		</Link>
	);
}
