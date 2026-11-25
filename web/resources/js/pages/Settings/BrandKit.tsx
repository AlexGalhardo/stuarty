import { Head } from "@inertiajs/react";
import { Check, Plus, Type, Upload, X } from "lucide-react";
import {
	type FormEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { LoadingState } from "@/components/LoadingState";
import { Notice } from "@/components/Notice";
import { PageHeader, PageHeaderAction } from "@/components/PageHeader";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { BrandFont, BrandKit, LibraryAsset } from "../../editor/api/types";
import { thumbnailUrl } from "../../editor/api/urls";
import { SELECT_CLASS, SettingsSection } from "./SettingsSection";

const WEIGHTS = [100, 200, 300, 400, 500, 600, 700, 800, 900] as const;
const NAME_MAX = 40;

interface ColorRow {
	id: number;
	name: string;
	value: string;
}

function fileName(path: string): string {
	return path.split(/[\\/]/).pop() ?? path;
}

// The label of an image of the workspace in the logo picker: its name and, when the service knows
// them, its dimensions.
export function logoLabel(asset: LibraryAsset): string {
	return asset.width && asset.height
		? `${asset.name} (${asset.width}x${asset.height})`
		: asset.name;
}

// The colour a row names, drawn as a small square so a person sees it. It is decoration: the value in the field
// is what is saved, and a value the browser cannot read leaves the square empty.
function Swatch({ value }: { value: string }): ReactNode {
	return (
		<span
			aria-hidden="true"
			data-slot="swatch"
			className="block h-16 w-full shrink-0 rounded-md border border-border-strong"
			style={{ backgroundColor: value }}
		/>
	);
}

// The brand kit of the workspace (SDD-012 section 12.1, SDD-005 section 5.11):
// a logo, named colours and font families, saved as a whole with one PUT. A
// font is imported by its path on the machine of the service (the operation
// takes a path, never the bytes, SDD-007 section 7.7); the file stays in the
// font store and the kit names it by hash.
export default function BrandKitPage(): ReactNode {
	const t = useT();
	const [loaded, setLoaded] = useState<"loading" | "failed" | "ready">(
		"loading",
	);
	const [failure, setFailure] = useState<Failure | null>(null);
	const [saved, setSaved] = useState(false);
	const [saving, setSaving] = useState(false);
	const [logo, setLogo] = useState("");
	const [images, setImages] = useState<readonly LibraryAsset[]>([]);
	const [colors, setColors] = useState<ColorRow[]>([]);
	const [fonts, setFonts] = useState<{ [family: string]: BrandFont }>({});
	const [names, setNames] = useState<{ [hash: string]: string }>({});
	const [nextId, setNextId] = useState(1);
	const [path, setPath] = useState("");
	const [family, setFamily] = useState("");
	const [weight, setWeight] = useState("400");
	const [importing, setImporting] = useState(false);

	const apply = useCallback((kit: BrandKit): void => {
		setLogo(kit.logo ?? "");
		setColors(
			Object.entries(kit.colors ?? {}).map(([name, value], index) => ({
				id: index,
				name,
				value,
			})),
		);
		setNextId(Object.keys(kit.colors ?? {}).length);
		setFonts(kit.fonts ?? {});
	}, []);

	useEffect(() => {
		let live = true;
		createClient(new HttpTransport("/api/v1"))
			.getBrandKit()
			.then((kit) => {
				if (!live) return;
				apply(kit);
				setLoaded("ready");
			})
			.catch(() => {
				if (live) setLoaded("failed");
			});
		// The picker is a convenience: a refused or failed list leaves the hash field alone (VV-117).
		createClient(new HttpTransport("/api/v1"))
			.listAssets("image")
			.then((result) => {
				if (live) setImages(result.assets);
			})
			.catch(() => {});
		return () => {
			live = false;
		};
	}, [apply]);

	function touch(): void {
		setSaved(false);
		setFailure(null);
	}

	function editColor(id: number, change: Partial<ColorRow>): void {
		touch();
		setColors((was) =>
			was.map((item) => (item.id === id ? { ...item, ...change } : item)),
		);
	}

	async function importFont(event: FormEvent<HTMLFormElement>): Promise<void> {
		event.preventDefault();
		setImporting(true);
		touch();
		try {
			const result = await createClient(
				new HttpTransport("/api/v1"),
			).importFont(path.trim());
			const name = family.trim();
			setNames((was) => ({ ...was, [result.hash]: fileName(path.trim()) }));
			setFonts((was) => {
				const files = (was[name]?.files ?? []).filter(
					(file) => file.weight !== Number(weight),
				);
				return {
					...was,
					[name]: {
						files: [...files, { weight: Number(weight), hash: result.hash }],
					},
				};
			});
			setPath("");
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setImporting(false);
		}
	}

	async function save(): Promise<void> {
		setSaving(true);
		touch();
		const body = {
			...(logo.trim() === "" ? {} : { logo: logo.trim() }),
			...(colors.length === 0
				? {}
				: {
						colors: Object.fromEntries(
							colors.map((row) => [row.name.trim(), row.value.trim()]),
						),
					}),
			...(Object.keys(fonts).length === 0 ? {} : { fonts }),
		};
		try {
			apply(await createClient(new HttpTransport("/api/v1")).setBrandKit(body));
			setSaved(true);
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setSaving(false);
		}
	}

	const logoAsset = images.find((a) => a.hash === logo);
	const sampleFont = Object.keys(fonts).sort()[0];

	return (
		<>
			<Head title={t("ui.brand.title")} />
			<div className="mx-auto flex max-w-6xl flex-col gap-6">
				<PageHeader
					title={t("ui.brand.title")}
					description={t("ui.brand.intro")}
					actions={
						loaded === "ready" ? (
							<PageHeaderAction
								type="button"
								disabled={saving}
								onClick={() => void save()}
							>
								<Check aria-hidden="true" />
								{t("ui.brand.save")}
							</PageHeaderAction>
						) : undefined
					}
				/>
				{loaded === "loading" && (
					<LoadingState message={t("ui.settings.loading")} />
				)}
				{loaded === "failed" && (
					<Notice
						tone="danger"
						role="alert"
						title={t("ui.settings.unavailable")}
					/>
				)}
				{loaded === "ready" && (
					<>
						<FailureNote failure={failure} />
						{saved && (
							<Notice
								tone="success"
								role="status"
								title={t("ui.brand.saved")}
							/>
						)}
						<div className="grid items-start gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
							<div className="flex min-w-0 flex-col gap-6">
								<SettingsSection id="brand-colors" title={t("ui.brand.colors")}>
									<p
										id="brand-color-name-hint"
										className="text-sm text-text-muted"
									>
										{t("ui.brand.color_name_hint")}
									</p>
									<ul className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
										{colors.map((row, index) => (
											<li
												key={row.id}
												className="flex flex-col gap-2"
												data-color-name={row.name}
											>
												<Swatch value={row.value} />
												<Input
													aria-label={t("ui.brand.color_name", {
														index: index + 1,
													})}
													value={row.name}
													placeholder={t("ui.brand.color_name_placeholder")}
													aria-describedby="brand-color-name-hint"
													maxLength={NAME_MAX}
													autoComplete="off"
													spellCheck={false}
													className="min-w-0 font-mono"
													onChange={(event) =>
														editColor(row.id, { name: event.target.value })
													}
												/>
												<div className="flex items-center gap-1">
													<Input
														aria-label={t("ui.brand.color_value", {
															index: index + 1,
														})}
														value={row.value}
														maxLength={9}
														autoComplete="off"
														spellCheck={false}
														className="min-w-0 flex-1 font-mono"
														onChange={(event) =>
															editColor(row.id, { value: event.target.value })
														}
													/>
													<Button
														type="button"
														variant="ghost"
														size="icon-sm"
														aria-label={t("ui.brand.color_remove", {
															name: row.name,
														})}
														onClick={() => {
															touch();
															setColors((was) =>
																was.filter((item) => item.id !== row.id),
															);
														}}
													>
														<X aria-hidden="true" />
													</Button>
												</div>
											</li>
										))}
									</ul>
									<div>
										<Button
											type="button"
											variant="outline"
											onClick={() => {
												touch();
												setColors((was) => [
													...was,
													{ id: nextId, name: "", value: "#" },
												]);
												setNextId((was) => was + 1);
											}}
										>
											<Plus aria-hidden="true" />
											{t("ui.brand.color_add")}
										</Button>
									</div>
								</SettingsSection>

								<SettingsSection id="brand-fonts" title={t("ui.brand.fonts")}>
									<ul className="flex flex-col gap-1" data-testid="brand-fonts">
										{Object.entries(fonts)
											.sort(([a], [b]) => a.localeCompare(b))
											.map(([name, font]) => (
												<li
													key={name}
													className="flex items-center gap-2 text-sm"
													data-font-family={name}
												>
													<Type
														aria-hidden="true"
														className="size-8 shrink-0 rounded-md bg-surface-hover p-1.5 text-icon"
													/>
													<span className="min-w-0 flex-1">
														<UntrustedText>{name}</UntrustedText>
														{[...font.files]
															.sort((a, b) => a.weight - b.weight)
															.map((file) => (
																<span
																	key={file.hash}
																	className="block text-xs text-text-muted"
																>
																	{file.weight}
																	{" · "}
																	<UntrustedText>
																		{names[file.hash] ?? file.hash.slice(0, 19)}
																	</UntrustedText>
																</span>
															))}
													</span>
													<Button
														type="button"
														variant="ghost"
														size="icon-sm"
														aria-label={t("ui.brand.font_remove", { name })}
														onClick={() => {
															touch();
															setFonts((was) =>
																Object.fromEntries(
																	Object.entries(was).filter(
																		([key]) => key !== name,
																	),
																),
															);
														}}
													>
														<X aria-hidden="true" />
													</Button>
												</li>
											))}
									</ul>
									<form
										onSubmit={(event) => void importFont(event)}
										className="flex flex-col gap-2"
									>
										<Label htmlFor="brand-font-path">
											{t("ui.brand.font_path")}
										</Label>
										<Input
											id="brand-font-path"
											value={path}
											autoComplete="off"
											spellCheck={false}
											onChange={(event) => setPath(event.target.value)}
										/>
										<div className="flex flex-wrap items-end gap-2">
											<div className="flex min-w-40 flex-1 flex-col gap-1">
												<Label htmlFor="brand-font-family">
													{t("ui.brand.font_family")}
												</Label>
												<Input
													id="brand-font-family"
													value={family}
													maxLength={60}
													autoComplete="off"
													onChange={(event) => setFamily(event.target.value)}
												/>
											</div>
											<div className="flex flex-col gap-1">
												<Label htmlFor="brand-font-weight">
													{t("ui.brand.font_weight")}
												</Label>
												<select
													id="brand-font-weight"
													value={weight}
													className={SELECT_CLASS}
													onChange={(event) => setWeight(event.target.value)}
												>
													{WEIGHTS.map((value) => (
														<option key={value} value={String(value)}>
															{value}
														</option>
													))}
												</select>
											</div>
											<Button
												type="submit"
												variant="outline"
												disabled={
													importing ||
													path.trim() === "" ||
													family.trim() === ""
												}
											>
												<Upload aria-hidden="true" />
												{t("ui.brand.font_import")}
											</Button>
										</div>
									</form>
								</SettingsSection>
							</div>
							<div className="flex min-w-0 flex-col gap-6">
								<SettingsSection id="brand-logo" title={t("ui.brand.logo")}>
									{logoAsset?.poster !== undefined && (
										<img
											src={thumbnailUrl(
												logoAsset.poster.timeline_id,
												logoAsset.poster.asset_id,
												160,
											)}
											alt=""
											data-testid="brand-logo-preview"
											className="h-16 w-full rounded-md bg-surface-hover object-contain"
										/>
									)}
									<div className="flex flex-col gap-2">
										<Label htmlFor="brand-logo-pick">
											{t("ui.brand.logo_pick")}
										</Label>
										<select
											id="brand-logo-pick"
											value={images.some((a) => a.hash === logo) ? logo : ""}
											disabled={images.length === 0}
											className={`${SELECT_CLASS} w-full`}
											onChange={(event) => {
												touch();
												setLogo(event.target.value);
											}}
										>
											<option value="">
												{images.length === 0
													? t("ui.brand.logo_none")
													: t("ui.brand.logo_choose")}
											</option>
											{images.map((asset) => (
												<option key={asset.hash} value={asset.hash}>
													{logoLabel(asset)}
												</option>
											))}
										</select>
									</div>
									<div className="flex flex-col gap-2">
										<Label htmlFor="brand-logo-hash">
											{t("ui.brand.logo_hash")}
										</Label>
										<Input
											id="brand-logo-hash"
											value={logo}
											placeholder="sha256:…"
											autoComplete="off"
											spellCheck={false}
											className="font-mono"
											onChange={(event) => {
												touch();
												setLogo(event.target.value);
											}}
										/>
										<p className="text-sm text-text-muted">
											{t("ui.brand.logo_hint")}
										</p>
									</div>
								</SettingsSection>

								<SettingsSection
									id="brand-preview"
									title={t("ui.brand.preview")}
								>
									<div
										data-testid="brand-preview"
										className="flex aspect-video flex-col justify-end gap-1 rounded-md bg-surface-sunken p-4 text-text"
										style={{
											backgroundColor: colors[0]?.value,
											color: colors[1]?.value,
											fontFamily: sampleFont,
										}}
									>
										<p className="text-2xl font-bold">
											{t("ui.brand.preview_title")}
										</p>
										<p className="text-base">{t("ui.brand.preview_byline")}</p>
									</div>
									<p className="text-sm text-text-muted">
										{t("ui.brand.preview_note")}
									</p>
								</SettingsSection>
							</div>
						</div>
					</>
				)}
			</div>
		</>
	);
}
