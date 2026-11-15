import { locales, localizePath } from "@/lib/locale";
import { ui } from "@/lib/ui-text";
import { withBase } from "@/lib/site";

/** The 404 page holds both languages (SDD-010 section 9); an inline script in 404.astro hides the one that does
 * not match the URL prefix. The language of an address that GitHub Pages cannot route is only known from its path. */
export function NotFoundView(): React.ReactElement {
	return (
		<main id="content" className="mx-auto max-w-xl px-4 py-24 text-center">
			<p className="text-sm font-medium text-accent">404</p>
			{locales.map((locale) => {
				const t = ui[locale];
				return (
					<div key={locale} data-lang={locale} lang={locale === "en" ? "en" : "pt-BR"}>
						<h1 className="mt-2 text-3xl font-semibold tracking-tight">{t.notFoundTitle}</h1>
						<p className="mt-3 text-muted">{t.notFoundBody}</p>
						<a
							href={withBase(localizePath(locale, "/docs/introduction/"))}
							className="mt-8 inline-flex h-10 items-center rounded-lg border border-border px-4 text-sm font-medium hover:bg-hover"
						>
							{t.goToIntroduction}
						</a>
					</div>
				);
			})}
		</main>
	);
}
