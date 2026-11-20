import { Link } from "@/components/link";
import { withBase } from "@/lib/site";
import { ui as t } from "@/lib/ui-text";

/** The Stuarty mark: the official icon, scaled from the source by internal/tools/installericon. */
export function LogoMark({ className = "size-6" }: { className?: string }) {
	return <img src={withBase("/logo.png")} alt="" width={24} height={24} className={className} />;
}

/** `lg` is the larger lockup of the expressive register (the landing pages). */
export function Logo({ size = "md" }: { size?: "md" | "lg" }) {
	const height = size === "lg" ? "h-7" : "h-6";
	return (
		<Link href="/" className="flex items-center rounded-md" aria-label={t.logoLabel}>
			<img
				src={withBase("/logo-wordmark-light.png")}
				alt=""
				width={112}
				height={24}
				className={`${height} w-auto dark:hidden`}
			/>
			<img
				src={withBase("/logo-wordmark-dark.png")}
				alt=""
				width={112}
				height={24}
				className={`hidden ${height} w-auto dark:block`}
			/>
		</Link>
	);
}
