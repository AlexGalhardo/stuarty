import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import type {
	Connection,
	CostSummary,
	ModelView,
	ProviderView,
} from "../../editor/api/types";
import ProviderCards, { ModelRow } from "./ProviderCards";

const PAYLOAD = "<img src=x onerror=window.__pwned=1>";

const messages = {
	"ui.providers.title": "AI providers",
	"ui.providers.intro": "Connect your own key.",
	"ui.providers.docs": "Where to find the key",
	"ui.providers.none": "Not connected yet.",
	"ui.providers.connect_title": "Connect {provider} with an API key",
	"ui.providers.connect_name": "Name for this connection",
	"ui.providers.connect_address": "Base address of the API",
	"ui.providers.connect_key": "API key",
	"ui.providers.connect_hint": "Stored encrypted.",
	"ui.providers.connect_submit": "Save the key",
	"ui.providers.check": "Check the key",
	"ui.providers.models": "Show the models",
	"ui.providers.limits":
		"Up to {context} tokens, {output} of them in the answer",
	"ui.providers.price":
		"Per million tokens: {input} in, {output} out ({source})",
	"ui.providers.price_none": "No price yet.",
	"ui.providers.source.user": "your price",
	"ui.providers.source.catalogue": "published by the provider",
	"ui.providers.source.none": "none",
	"ui.providers.set_price": "Set the price",
	"ui.providers.spend": "Spent today: {amount}",
	"ui.providers.spend_none": "Nothing spent today.",
};

const provider: ProviderView = {
	id: "openai",
	name: "OpenAI",
	capabilities: ["text"],
	credential_kinds: ["api_key"],
	default_endpoint: "https://api.openai.com/v1",
	docs_url: "https://platform.openai.com/api-keys",
	connections: ["con_1"],
};

const connection: Connection = {
	id: "con_1",
	kind: "openai",
	label: PAYLOAD,
	endpoint: "https://api.openai.com/v1",
	hosts: ["api.openai.com"],
	accepts: ["text"],
	created_at: "2026-11-19T12:00:00Z",
	last_used_at: null,
	secret_set: true,
};

const model = (extra: Partial<ModelView> = {}): ModelView => ({
	id: "a-model",
	name: "A model",
	context_tokens: 0,
	output_tokens: 0,
	price: null,
	price_source: "none",
	...extra,
});

function wrap(node: React.ReactNode): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			{node}
		</I18nProvider>,
	);
}

const cost = (micros: number): CostSummary => ({
	ceilings: [],
	today: [],
	day: "2026-11-19",
	days: [
		{
			day: "2026-11-19",
			connection_id: "con_1",
			currency: "USD",
			micros,
		},
		{
			day: "2026-11-18",
			connection_id: "con_1",
			currency: "USD",
			micros: 999_000_000,
		},
		{
			day: "2026-11-19",
			connection_id: "con_2",
			currency: "USD",
			micros: 888_000_000,
		},
	],
});

// SDD-007 section 8.5, threat T-22: the label of a connection and the name of a model are text from a
// person and from the network; both reach the page as text, never as markup.
describe("the provider cards render text from people and from the network as text", () => {
	test("the label of a connection and the name of a model", () => {
		const html = wrap(
			<ProviderCards
				providers={[provider]}
				connections={[connection]}
				cost={null}
				onChanged={() => undefined}
			/>,
		);
		expect(html).toContain("&lt;img src=x onerror=window.__pwned=1&gt;");
		expect(html).not.toContain("<img");

		const row = wrap(
			<ul>
				<ModelRow
					model={model({ name: PAYLOAD })}
					onPrice={() => Promise.resolve()}
				/>
			</ul>,
		);
		expect(row).toContain("&lt;img");
		expect(row).not.toContain("<img");
	});
});

describe("a provider card", () => {
	test("names the provider, links to where the key is, and has a form that takes a key without showing it", () => {
		const html = wrap(
			<ProviderCards
				providers={[provider]}
				connections={[connection]}
				cost={null}
				onChanged={() => undefined}
			/>,
		);
		expect(html).toContain("OpenAI");
		expect(html).toContain("https://platform.openai.com/api-keys");
		expect(html).toContain('type="password"');
		expect(html).toContain("https://api.openai.com/v1");
		expect(html).toContain("Save the key");
	});

	test("says so when there is no connection yet", () => {
		const html = wrap(
			<ProviderCards
				providers={[{ ...provider, connections: [] }]}
				connections={[]}
				cost={null}
				onChanged={() => undefined}
			/>,
		);
		expect(html).toContain("Not connected yet.");
	});

	test("shows the spend of today of the connection and nothing of other days or connections", () => {
		const html = wrap(
			<ProviderCards
				providers={[provider]}
				connections={[connection]}
				cost={cost(1_500_000)}
				onChanged={() => undefined}
			/>,
		);
		expect(html).toContain("Spent today:");
		expect(html).toContain("1.50");
		expect(html).not.toContain("999");
		expect(html).not.toContain("888");
	});

	test("says nothing was spent when no line of today is the connection's", () => {
		const html = wrap(
			<ProviderCards
				providers={[provider]}
				connections={[connection]}
				cost={{ ...cost(0), days: [] }}
				onChanged={() => undefined}
			/>,
		);
		expect(html).toContain("Nothing spent today.");
	});
});

describe("a model row", () => {
	test("says a model has no price, and where a price comes from", () => {
		expect(
			wrap(
				<ul>
					<ModelRow model={model()} onPrice={() => Promise.resolve()} />
				</ul>,
			),
		).toContain("No price yet.");

		const priced = wrap(
			<ul>
				<ModelRow
					model={model({
						price: {
							currency: "USD",
							input_micros_per_mtok: 2_500_000,
							output_micros_per_mtok: 10_000_000,
						},
						price_source: "user",
						context_tokens: 128000,
						output_tokens: 16000,
					})}
					onPrice={() => Promise.resolve()}
				/>
			</ul>,
		);
		expect(priced).toContain("2.50");
		expect(priced).toContain("10.00");
		expect(priced).toContain("your price");
		expect(priced).toContain("128,000");
		expect(priced).not.toContain("No price yet.");
	});
});
