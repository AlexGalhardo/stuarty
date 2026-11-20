import { Head } from "@inertiajs/react";
import { X } from "lucide-react";
import {
	type FormEvent,
	type ReactNode,
	useCallback,
	useEffect,
	useState,
} from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { BrandFont, BrandKit, LibraryAsset } from "../../editor/api/types";

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

	return (
		<>
			<Head title={t("ui.brand.title")} />
			<div className="mx-auto flex max-w-2xl flex-col gap-8">
				<h1 className="text-2xl font-semibold">{t("ui.brand.title")}</h1>
				<p className="text-sm text-muted-foreground">{t("ui.brand.intro")}</p>
				{loaded === "loading" && (
					<p className="text-sm text-muted-foreground">
						{t("ui.settings.loading")}
					</p>
				)}
				{loaded === "failed" && (
					<p role="alert" className="text-sm text-destructive">
						{t("ui.settings.unavailable")}
					</p>
				)}
				{loaded === "ready" && (
					<>
						<section
							aria-labelledby="brand-logo"
							className="flex flex-col gap-2"
						>
							<h2 id="brand-logo" className="text-lg font-semibold">
								{t("ui.brand.logo")}
							</h2>
							<Label htmlFor="brand-logo-pick">{t("ui.brand.logo_pick")}</Label>
							<select
								id="brand-logo-pick"
								value={images.some((a) => a.hash === logo) ? logo : ""}
								disabled={images.length === 0}
								className="h-8 rounded-lg border border-input bg-transparent px-2.5 text-sm dark:bg-input/30"
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
							<Label htmlFor="brand-logo-hash">{t("ui.brand.logo_hash")}</Label>
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
							<p className="text-xs text-muted-foreground">
								{t("ui.brand.logo_hint")}
							</p>
						</section>

						<section
							aria-labelledby="brand-colors"
							className="flex flex-col gap-3"
						>
							<h2 id="brand-colors" className="text-lg font-semibold">
								{t("ui.brand.colors")}
							</h2>
							<p
								id="brand-color-name-hint"
								className="text-sm text-muted-foreground"
							>
								{t("ui.brand.color_name_hint")}
							</p>
							<ul className="flex flex-col gap-2">
								{colors.map((row, index) => (
									<li
										key={row.id}
										className="flex items-center gap-2"
										data-color-name={row.name}
									>
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
											className="h-8 flex-1 font-mono"
											onChange={(event) =>
												editColor(row.id, { name: event.target.value })
											}
										/>
										<Input
											aria-label={t("ui.brand.color_value", {
												index: index + 1,
											})}
											value={row.value}
											maxLength={9}
											autoComplete="off"
											spellCheck={false}
											className="h-8 w-28 font-mono"
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
									{t("ui.brand.color_add")}
								</Button>
							</div>
						</section>

						<section
							aria-labelledby="brand-fonts"
							className="flex flex-col gap-3"
						>
							<h2 id="brand-fonts" className="text-lg font-semibold">
								{t("ui.brand.fonts")}
							</h2>
							<ul className="flex flex-col gap-1" data-testid="brand-fonts">
								{Object.entries(fonts)
									.sort(([a], [b]) => a.localeCompare(b))
									.map(([name, font]) => (
										<li
											key={name}
											className="flex items-center gap-2 text-sm"
											data-font-family={name}
										>
											<span className="min-w-0 flex-1">
												<UntrustedText>{name}</UntrustedText>
												{[...font.files]
													.sort((a, b) => a.weight - b.weight)
													.map((file) => (
														<span
															key={file.hash}
															className="block text-xs text-muted-foreground"
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
								<div className="flex items-end gap-2">
									<div className="flex flex-1 flex-col gap-1">
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
											className="h-8 rounded-lg border border-input bg-transparent px-2.5 text-sm dark:bg-input/30"
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
											importing || path.trim() === "" || family.trim() === ""
										}
									>
										{t("ui.brand.font_import")}
									</Button>
								</div>
							</form>
						</section>

						<div className="flex flex-col gap-2">
							<FailureNote failure={failure} />
							{saved && (
								<p role="status" className="text-sm text-success">
									{t("ui.brand.saved")}
								</p>
							)}
							<div>
								<Button
									type="button"
									disabled={saving}
									onClick={() => void save()}
								>
									{t("ui.brand.save")}
								</Button>
							</div>
						</div>
					</>
				)}
			</div>
		</>
	);
}
