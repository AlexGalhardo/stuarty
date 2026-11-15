import { Plus, X } from "lucide-react";
import { type ReactNode, useState } from "react";

import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Textarea } from "@/components/ui/textarea";
import { useT } from "@/lib/i18n";
import type { JsonValue } from "../../api/types";
import TimeField from "../../ui/TimeField";
import { localizeDecimalInput, normalizeTimeInput } from "../units";
import { type Construct, classify, type Schema } from "./classify";
import { useForm } from "./context";
import type { Draft } from "./draft";

export interface NodeProps {
	schema: Schema;
	name: string;
	// `args.start`, `args.words[0].start`: the path of the core's errors.
	path: string;
	draft: Draft;
	required: boolean;
	onChange(draft: Draft): void;
}

const SELECT =
	"h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2.5 text-sm outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 aria-invalid:border-destructive dark:bg-input/30";

const descriptionOf = (schema: Schema): string | undefined =>
	typeof schema.description === "string" && schema.description !== ""
		? schema.description
		: undefined;

// The error of the core that names this field, shown under it.
function FieldError({ path }: { path: string }): ReactNode {
	const { error } = useForm();
	if (error === null || error.path !== path) return null;
	return (
		<p role="alert" lang="en" className="text-xs text-destructive">
			<UntrustedText>{error.message}</UntrustedText>
			{error.hint !== undefined && (
				<>
					{" "}
					<UntrustedText>{error.hint}</UntrustedText>
				</>
			)}
		</p>
	);
}

function FieldLabel({
	htmlFor,
	name,
	required,
}: {
	htmlFor: string;
	name: string;
	required: boolean;
}): ReactNode {
	const t = useT();
	return (
		<Label htmlFor={htmlFor} className="font-mono text-xs">
			{name}
			{required && (
				<>
					<span aria-hidden="true"> *</span>
					<span className="sr-only"> ({t("ui.editor.palette.required")})</span>
				</>
			)}
		</Label>
	);
}

// A label on the left and the control with its hint and error on the right
// (docs/design/alpha-editor.md section 2.2).
function Row({
	id,
	name,
	required,
	schema,
	path,
	children,
}: {
	id: string;
	name: string;
	required: boolean;
	schema: Schema;
	path: string;
	children: ReactNode;
}): ReactNode {
	const hint = descriptionOf(schema);
	return (
		<div className="grid grid-cols-[128px_1fr] items-start gap-x-3 gap-y-1">
			<div className="pt-1.5">
				<FieldLabel htmlFor={id} name={name} required={required} />
			</div>
			<div className="flex min-w-0 flex-col gap-1">
				{children}
				{hint !== undefined && (
					<p lang="en" className="text-xs text-muted-foreground">
						{hint}
					</p>
				)}
				<FieldError path={path} />
			</div>
		</div>
	);
}

function TimeNode({ draft, onChange, ...props }: NodeProps): ReactNode {
	const { locale, prefix } = useForm();
	const [touched, setTouched] = useState(false);
	const text = typeof draft === "string" ? draft : "";
	const invalid =
		touched &&
		text.trim() !== "" &&
		!normalizeTimeInput(localizeDecimalInput(text, locale)).ok;
	const id = `${prefix}${props.path}`;
	return (
		<Row id={id} {...props}>
			<TimeField
				id={id}
				data-path={props.path}
				value={text}
				invalid={invalid}
				onValueChange={(value) => onChange(value)}
				onBlur={() => setTouched(true)}
			/>
		</Row>
	);
}

function IdNode({
	construct,
	draft,
	onChange,
	...props
}: NodeProps & { construct: Extract<Construct, { kind: "id" }> }): ReactNode {
	const { options, prefix } = useForm();
	const id = `${prefix}${props.path}`;
	const listId = `${id}-options`;
	return (
		<Row id={id} {...props}>
			<Input
				id={id}
				data-path={props.path}
				list={listId}
				autoComplete="off"
				spellCheck={false}
				className="font-mono"
				value={typeof draft === "string" ? draft : ""}
				onChange={(event) => onChange(event.target.value)}
			/>
			<datalist id={listId}>
				{options[construct.idKind].map((option) => (
					<option key={option.id} value={option.id}>
						{option.label}
					</option>
				))}
			</datalist>
		</Row>
	);
}

function EnumNode({
	construct,
	draft,
	onChange,
	...props
}: NodeProps & { construct: Extract<Construct, { kind: "enum" }> }): ReactNode {
	const t = useT();
	const { prefix } = useForm();
	const id = `${prefix}${props.path}`;
	const selected = construct.values.indexOf(draft as JsonValue);
	return (
		<Row id={id} {...props}>
			<select
				id={id}
				data-path={props.path}
				className={SELECT}
				value={selected < 0 ? "" : String(selected)}
				onChange={(event) => {
					const index = event.target.value;
					onChange(
						index === ""
							? undefined
							: (construct.values[Number(index)] as Draft),
					);
				}}
			>
				<option value="">{t("ui.editor.palette.unset")}</option>
				{construct.values.map((value, index) => (
					<option key={JSON.stringify(value)} value={String(index)}>
						{String(value)}
					</option>
				))}
			</select>
		</Row>
	);
}

function NumberNode({
	construct,
	draft,
	onChange,
	...props
}: NodeProps & {
	construct: Extract<Construct, { kind: "number" }>;
}): ReactNode {
	const t = useT();
	const { prefix } = useForm();
	const id = `${prefix}${props.path}`;
	const bounded = construct.min !== undefined || construct.max !== undefined;
	return (
		<Row id={id} {...props}>
			<Input
				id={id}
				data-path={props.path}
				type="number"
				step={construct.integer ? 1 : "any"}
				min={construct.min}
				max={construct.max}
				value={typeof draft === "number" ? draft : ""}
				onChange={(event) =>
					onChange(
						Number.isNaN(event.target.valueAsNumber)
							? undefined
							: event.target.valueAsNumber,
					)
				}
			/>
			{bounded && (
				<p className="text-xs text-muted-foreground">
					{t("ui.editor.palette.range", {
						min: construct.min ?? "-",
						max: construct.max ?? "-",
					})}
				</p>
			)}
		</Row>
	);
}

function BooleanNode({ draft, onChange, ...props }: NodeProps): ReactNode {
	const { prefix } = useForm();
	const id = `${prefix}${props.path}`;
	return (
		<Row id={id} {...props}>
			<Checkbox
				id={id}
				data-path={props.path}
				checked={
					typeof draft === "boolean" ? draft : props.schema.default === true
				}
				onCheckedChange={(checked) => onChange(checked === true)}
			/>
		</Row>
	);
}

function TextNode({ draft, onChange, ...props }: NodeProps): ReactNode {
	const { prefix } = useForm();
	const id = `${prefix}${props.path}`;
	return (
		<Row id={id} {...props}>
			<Input
				id={id}
				data-path={props.path}
				value={typeof draft === "string" ? draft : ""}
				onChange={(event) => onChange(event.target.value)}
			/>
		</Row>
	);
}

function JsonNode({ draft, onChange, ...props }: NodeProps): ReactNode {
	const t = useT();
	const { prefix } = useForm();
	const id = `${prefix}${props.path}`;
	const text = typeof draft === "string" ? draft : "";
	let invalid = false;
	if (text.trim() !== "") {
		try {
			JSON.parse(text);
		} catch {
			invalid = true;
		}
	}
	return (
		<Row id={id} {...props}>
			<Textarea
				id={id}
				data-path={props.path}
				data-fallback="json"
				className="min-h-20 font-mono text-xs"
				spellCheck={false}
				aria-invalid={invalid || undefined}
				value={text}
				onChange={(event) => onChange(event.target.value)}
			/>
			{invalid && (
				<p role="alert" className="text-xs text-destructive">
					{t("ui.editor.palette.invalid_json")}
				</p>
			)}
		</Row>
	);
}

// A group of fields: a fieldset with a legend and a leading line.
function Group({
	legend,
	path,
	children,
}: {
	legend: string;
	path: string;
	children: ReactNode;
}): ReactNode {
	return (
		<fieldset
			data-path={path}
			className="flex flex-col gap-2 border-l border-border pl-4"
		>
			<legend className="-ml-4 px-4 font-mono text-xs">{legend}</legend>
			{children}
		</fieldset>
	);
}

function ObjectFields({
	construct,
	path,
	draft,
	onChange,
}: {
	construct: Extract<Construct, { kind: "object" }>;
	path: string;
	draft: Draft;
	onChange(draft: Draft): void;
}): ReactNode {
	const record =
		typeof draft === "object" && draft !== null && !Array.isArray(draft)
			? (draft as { [key: string]: Draft })
			: {};
	return (
		<>
			{construct.fields.map((field) => (
				<FormNode
					key={field.name}
					schema={field.schema}
					name={field.name}
					path={`${path}.${field.name}`}
					required={field.required}
					draft={record[field.name]}
					onChange={(value) => {
						const next = { ...record };
						if (value === undefined) delete next[field.name];
						else next[field.name] = value;
						onChange(next);
					}}
				/>
			))}
		</>
	);
}

function ListNode({
	construct,
	draft,
	onChange,
	...props
}: NodeProps & { construct: Extract<Construct, { kind: "list" }> }): ReactNode {
	const t = useT();
	const rows = Array.isArray(draft) ? draft : [];
	const objectRows = construct.items.type === "object";
	const set = (next: Draft[]): void => onChange(next);
	const hint = descriptionOf(props.schema);
	return (
		<Group legend={props.name} path={props.path}>
			{hint !== undefined && (
				<p lang="en" className="text-xs text-muted-foreground">
					{hint}
				</p>
			)}
			<ul className="flex flex-col gap-2">
				{rows.map((row, index) => (
					<li
						// biome-ignore lint/suspicious/noArrayIndexKey: rows have no identity of their own
						key={index}
						className="flex items-start gap-2"
					>
						<div className="flex min-w-0 flex-1 flex-col gap-2">
							<FormNode
								schema={construct.items}
								name={construct.itemKey}
								path={`${props.path}[${index}]`}
								required={false}
								draft={row}
								inline
								onChange={(value) =>
									set(rows.map((r, i) => (i === index ? value : r)))
								}
							/>
						</div>
						<Button
							type="button"
							size="icon-sm"
							variant="ghost"
							aria-label={t("ui.editor.palette.remove_value", { n: index + 1 })}
							onClick={() => set(rows.filter((_, i) => i !== index))}
						>
							<X aria-hidden="true" />
						</Button>
					</li>
				))}
			</ul>
			<Button
				type="button"
				size="sm"
				variant="outline"
				className="self-start"
				onClick={() => set([...rows, objectRows ? {} : undefined])}
			>
				<Plus aria-hidden="true" />
				{t("ui.editor.palette.add_value")}
			</Button>
			<FieldError path={props.path} />
		</Group>
	);
}

function VariantsNode({
	construct,
	draft,
	onChange,
	...props
}: NodeProps & {
	construct: Extract<Construct, { kind: "variants" }>;
}): ReactNode {
	const t = useT();
	const { prefix } = useForm();
	const chosen =
		typeof draft === "object" &&
		draft !== null &&
		!Array.isArray(draft) &&
		"variant" in draft
			? (draft as { variant: number; value: Draft })
			: null;
	const label = (variant: Schema, index: number): string =>
		typeof variant.title === "string"
			? variant.title
			: (descriptionOf(variant) ??
				t("ui.editor.palette.variant", { n: index + 1 }));
	const variant =
		chosen === null ? undefined : construct.variants[chosen.variant];
	return (
		<Group legend={props.name} path={props.path}>
			<RadioGroup
				aria-label={props.name}
				value={chosen === null ? "" : String(chosen.variant)}
				onValueChange={(value) =>
					onChange({ variant: Number(value), value: undefined })
				}
			>
				{construct.variants.map((candidate, index) => {
					const id = `${prefix}${props.path}-variant-${index}`;
					return (
						// biome-ignore lint/suspicious/noArrayIndexKey: variants are positional
						<div key={index} className="flex items-center gap-2">
							<RadioGroupItem id={id} value={String(index)} />
							<Label htmlFor={id} lang="en">
								{label(candidate, index)}
							</Label>
						</div>
					);
				})}
			</RadioGroup>
			{chosen !== null && variant !== undefined && (
				<FormNode
					schema={variant}
					name={props.name}
					path={props.path}
					required={props.required}
					draft={chosen.value}
					onChange={(value) => onChange({ variant: chosen.variant, value })}
					inline
				/>
			)}
		</Group>
	);
}

// One renderer per construct of the table of SDD-012 section 9.2. An `inline`
// object (a row of a list, a chosen variant) has no group of its own.
export function FormNode(props: NodeProps & { inline?: boolean }): ReactNode {
	const { inline, ...node } = props;
	const construct = classify(node.schema, node.name);
	switch (construct.kind) {
		case "time":
			return <TimeNode {...node} />;
		case "id":
			return <IdNode {...node} construct={construct} />;
		case "enum":
			return <EnumNode {...node} construct={construct} />;
		case "number":
			return <NumberNode {...node} construct={construct} />;
		case "boolean":
			return <BooleanNode {...node} />;
		case "text":
			return <TextNode {...node} />;
		case "json":
			return <JsonNode {...node} />;
		case "list":
			return <ListNode {...node} construct={construct} />;
		case "variants":
			return <VariantsNode {...node} construct={construct} />;
		case "object":
			return inline === true ? (
				<ObjectFields
					construct={construct}
					path={node.path}
					draft={node.draft}
					onChange={node.onChange}
				/>
			) : (
				<Group legend={node.name} path={node.path}>
					<ObjectFields
						construct={construct}
						path={node.path}
						draft={node.draft}
						onChange={node.onChange}
					/>
					<FieldError path={node.path} />
				</Group>
			);
	}
}
