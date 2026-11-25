import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import ToggleField from "./ToggleField";

const messages = {
	"ui.editor.toggle.on": "Ligado",
	"ui.editor.toggle.off": "Desligado",
};

function html(stored: boolean): string {
	return renderToString(
		<I18nProvider locale="pt-BR" messages={messages}>
			<ToggleField
				label="Ativo"
				stored={stored}
				commit={async () => ({ ok: true })}
			/>
		</I18nProvider>,
	);
}

describe("ToggleField", () => {
	test("stays a native checkbox with its label and says its state in a word", () => {
		const on = html(true);
		expect(on).toContain('type="checkbox"');
		expect(on).toContain("checked");
		expect(on).toContain("Ativo");
		expect(on).toContain("Ligado");
		const off = html(false);
		expect(off).toContain("Desligado");
	});
});
