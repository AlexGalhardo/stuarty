import { Head } from "@inertiajs/react";
import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import ImageEditor from "../../image-editor/ImageEditor";

// The editor of one image document (SDD-012 section 24.1). The controller sends only the id; the editor reads
// the document through the API.
export default function Show({ imageId }: { imageId: string }): ReactNode {
	const t = useT();

	return (
		<>
			<Head title={t("ui.image.editor.title")} />
			<ImageEditor imageId={imageId} />
		</>
	);
}
