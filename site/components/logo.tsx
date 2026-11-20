import { Link } from "@/components/link";
import { withBase } from "@/lib/site";
import { ui as t } from "@/lib/ui-text";

/** The Stuarty mark: the official icon, scaled from the source by internal/tools/installericon. */
export function LogoMark({ className = "size-6" }: { className?: string }) {
	return <img src={withBase("/logo.png")} alt="" width={24} height={24} className={className} />;
}

export function Logo() {
	return (
		<Link href="/" className="flex items-center rounded-md" aria-label={t.logoLabel}>
			<img
				src={withBase("/logo-wordmark-light.png")}
				alt=""
				width={112}
				height={24}
				className="h-6 w-auto dark:hidden"
			/>
			<img
				src={withBase("/logo-wordmark-dark.png")}
				alt=""
				width={112}
				height={24}
				className="hidden h-6 w-auto dark:block"
			/>
		</Link>
	);
}
