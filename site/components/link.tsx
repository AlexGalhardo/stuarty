import type { ComponentProps } from "react";
import { useLocale } from "@/lib/locale-context";
import { localizePath } from "@/lib/locale";
import { withBase } from "@/lib/site";
import { withTrailingSlash } from "@/lib/trailing-slash";

/** A plain anchor. The docs site is static HTML, so there is no client router; internal paths get
 * the trailing slash the pages are served at and the base path of the deployment. */
export function Link({ href, ...rest }: ComponentProps<"a">) {
	const locale = useLocale();
	// A page links to /docs/...; in a Portuguese page the link goes to the Portuguese tree (SDD-010 section 5.1).
	return <a href={href === undefined ? href : withBase(withTrailingSlash(localizePath(locale, href)))} {...rest} />;
}
