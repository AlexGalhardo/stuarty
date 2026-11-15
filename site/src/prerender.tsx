import { prerender } from "react-dom/static";
import { ChangelogPage } from "@/components/changelog-page";
import { DocsFrame } from "@/components/docs-frame";
import { DocArticle } from "@/components/doc-article";
import { LocaleProvider } from "@/lib/locale-context";
import type { Doc } from "@/lib/content";
import type { Locale } from "@/lib/locale";

async function markup(node: React.ReactNode): Promise<string> {
	const { prelude } = await prerender(node);
	return new Response(prelude).text();
}

/** Docs frame and article as HTML. Interactive tabs and copy are plain DOM. */
export function renderDoc(doc: Doc): Promise<string> {
	return markup(
		<LocaleProvider locale={doc.locale}>
			<DocsFrame pathname={doc.href} locale={doc.locale}>
				<DocArticle doc={doc} />
			</DocsFrame>
		</LocaleProvider>,
	);
}

export function renderChangelog(locale: Locale): Promise<string> {
	return markup(
		<LocaleProvider locale={locale}>
			<ChangelogPage locale={locale} />
		</LocaleProvider>,
	);
}
