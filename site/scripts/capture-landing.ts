// Captures the product images of the landing page (epic 95) from a real `stuarty serve`, so that they can be made again.
//
//   bun scripts/capture-landing.ts --bin <stuarty binary> --work <empty folder> [--port 9751]
//                                  [--phase all|seed|shoot|encode] [--only <screen>,<screen>]
//
// It starts the server on a throwaway workspace inside --work (never ~/.stuarty), seeds the demo of the landing with the
// CLI and the HTTP API of the product (the user Alex, the video project "Reels 5 filming mistakes", the text project
// "Post natural light at home", the image project and its thumbnail, their media, a blog folder, a pending voice-over,
// some posts on the calendar, a skill, a prompt and a memory note), takes
// the screens in both themes with headless Chromium and encodes them to site/public/landing as .webp and .avif at 1600 and
// 960 px with the pinned LGPL FFmpeg. Nothing is drawn: every picture is a screenshot of the UI. The invented content
// names no real person, brand or number. Playwright is the one of web/ (run `bun install` there first).
//
// Environment: STUARTY_FFMPEG and STUARTY_FFPROBE name the FFmpeg of the demo media and of the encoding. Every other
// STUARTY_* and VUKEM_VIDEO_* variable of the caller is dropped, so the CLI can never reach the user's own workspace or
// server (STUARTY_URL, STUARTY_KEY, STUARTY_PROJECT), and the vault key of the throwaway workspace is a file inside
// --work instead of an item of the operating system keychain.
import { type ChildProcess, execFileSync, spawn } from "node:child_process";
import { existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { homedir } from "node:os";
import { dirname, join, parse, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const webDir = resolve(here, "../../web");
const publicLanding = resolve(here, "../public/landing");
const require = createRequire(join(webDir, "package.json"));
const { chromium } = require("@playwright/test") as typeof import("@playwright/test");
type Page = import("@playwright/test").Page;
type BrowserContext = import("@playwright/test").BrowserContext;

function arg(name: string, fallback?: string): string {
	const i = process.argv.indexOf(`--${name}`);
	const value = i >= 0 ? process.argv[i + 1] : fallback;
	if (value === undefined) throw new Error(`missing --${name}`);
	return value;
}

const bin = resolve(arg("bin"));
const work = resolve(arg("work"));
const port = Number(arg("port", "9751"));
const phase = arg("phase", "all");
// --only a,b shoots and encodes only those screens, to retake one without the others.
const only = arg("only", "").split(",").filter(Boolean);
const ffmpeg = process.env.STUARTY_FFMPEG ?? "ffmpeg";
const home = join(work, "ws");
const media = join(work, "media");
const shots = join(work, "shots");
const base = `http://127.0.0.1:${port}`;

// --work is emptied by the seed phase, so it must be a folder of its own: never a root, the home folder, the
// repository, a workspace of the product or a folder that holds one, and never a folder with files this script did not
// make.
const MARKER = ".capture-landing";
function checkWork(): void {
	const inside = (child: string, parent: string): boolean => {
		const r = relative(parent, child);
		return r === "" || (!r.startsWith("..") && !parse(r).root);
	};
	const repo = resolve(here, "../..");
	const workspaces = [join(homedir(), ".stuarty"), join(homedir(), ".vukem-video")];
	if (work === parse(work).root || work === resolve(homedir()) || inside(repo, work) || workspaces.some((w) => inside(w, work) || inside(work, w))) {
		throw new Error(`--work ${work} is not a throwaway folder: name an empty folder of its own`);
	}
	if (existsSync(work) && readdirSync(work).length > 0 && !existsSync(join(work, MARKER))) {
		throw new Error(`--work ${work} holds files this script did not make: name an empty folder`);
	}
}

const env: Record<string, string | undefined> = Object.fromEntries(
	Object.entries(process.env).filter(([k]) => !/^(STUARTY|VUKEM_VIDEO)_/i.test(k) || k === "STUARTY_FFMPEG" || k === "STUARTY_FFPROBE"),
);
Object.assign(env, {
	STUARTY_HOME: home,
	STUARTY_ADDR: `127.0.0.1:${port}`,
	STUARTY_MIN_FREE_BYTES: "1",
	STUARTY_MASTER_KEY_FILE: join(work, "master.key"),
});
const PASSWORD = "Demo-Password-1!";
const JSON_HEADERS = { "Content-Type": "application/json" };
// A person decides and prices from the page, so the product expects what a browser sends from its own origin.
const PERSON_HEADERS = { ...JSON_HEADERS, "Sec-Fetch-Site": "same-origin" };

function cli(args: string[], input?: string): string {
	return execFileSync(bin, args, { env, input, encoding: "utf8", stdio: ["pipe", "pipe", "pipe"] });
}
function cliJson<T>(args: string[]): T {
	return JSON.parse(cli([...args, "--json"])) as T;
}

const ALPHABET = "0123456789abcdefghjkmnpqrstvwxyz";
function newId(prefix: string): string {
	let out = "";
	for (let i = 0; i < 10; i++) out += ALPHABET[Math.floor(Math.random() * 32)];
	return `${prefix}_${out}`;
}

let server: ChildProcess | undefined;

async function startServer(): Promise<void> {
	rmSync(home, { recursive: true, force: true });
	mkdirSync(home, { recursive: true });
	server = spawn(bin, ["serve"], { env, stdio: "ignore" });
	for (let i = 0; i < 120; i++) {
		try {
			if ((await fetch(`${base}/auth/account`)).ok) return;
		} catch {
			// not listening yet
		}
		await new Promise((r) => setTimeout(r, 500));
	}
	throw new Error("the server did not start");
}

function stopServer(): void {
	if (!server?.pid) return;
	if (process.platform === "win32") execFileSync("taskkill", ["/PID", String(server.pid), "/T", "/F"], { stdio: "ignore" });
	else server.kill("SIGTERM");
	server = undefined;
}

// The demo media: gradients and sine tones made with the project's own FFmpeg. No footage, no recording of anyone.
function makeMedia(): void {
	mkdirSync(media, { recursive: true });
	const clip = (name: string, c0: string, c1: string, seconds: number, hz: number): void => {
		execFileSync(ffmpeg, ["-v", "error", "-y", "-f", "lavfi", "-i", `gradients=s=1080x1920:d=${seconds}:c0=${c0}:c1=${c1}:speed=0.02:r=30`, "-f", "lavfi", "-i", `sine=f=${hz}:d=${seconds}`, "-c:v", "libopenh264", "-b:v", "3M", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", join(media, `${name}.mp4`)]);
	};
	clip("take-facing-window", "0x3D2150", "0xE8795A", 6, 220);
	clip("broll-microphone", "0x14233F", "0x5B9BD5", 4, 260);
	clip("alex-intro", "0x16362B", "0x9CC45A", 5, 300);
	execFileSync(ffmpeg, ["-v", "error", "-y", "-f", "lavfi", "-i", "sine=f=330:d=20", "-c:a", "aac", join(media, "voice-over.m4a")]);
	execFileSync(ffmpeg, ["-v", "error", "-y", "-f", "lavfi", "-i", "sine=f=196:d=40", "-c:a", "libmp3lame", join(media, "calm-track.mp3")]);
	const still = (name: string, size: string, c0: string, c1: string): void => {
		execFileSync(ffmpeg, ["-v", "error", "-y", "-f", "lavfi", "-i", `gradients=s=${size}:c0=${c0}:c1=${c1}:speed=0`, "-frames:v", "1", join(media, `${name}.png`)]);
	};
	still("window-light", "1080x1350", "0xF2C98A", "0x5B7FB5");
	still("phone-on-books", "1080x1350", "0x2E4A3B", "0xC9A66B");
	still("thumbnail-background", "1280x720", "0x3D2150", "0xE8795A");
}

async function seed(context: BrowserContext): Promise<{ video: string; text: string; trailText: string }> {
	const api = context.request;
	// The account of the demo: the user Alex, made the way the product makes it (a launch code, then the account).
	const token = readFileSync(join(home, "token"), "utf8").trim();
	const launch = await api.post("/api/v1/system/launch", { headers: { ...JSON_HEADERS, Authorization: `Bearer ${token}` } });
	const { code } = (await launch.json()) as { code: string };
	await api.get(`/auth/launch?code=${code}`, { headers: { "Sec-Fetch-Site": "none" }, maxRedirects: 0 });
	const created = await api.post("/auth/account/create", {
		headers: { ...JSON_HEADERS, "Sec-Fetch-Site": "same-origin" },
		data: { username: "alex", email: "alex@example.test", password: PASSWORD, confirmation: PASSWORD },
	});
	if (!created.ok()) throw new Error(`account: ${await created.text()}`);
	const login = await api.post("/auth/account/login", { headers: JSON_HEADERS, data: { identity: "alex", password: PASSWORD } });
	if (!login.ok()) throw new Error(`login: ${await login.text()}`);

	const goal = (specific: string, measurable: string): string[] => [
		"--specific", specific,
		"--measurable", measurable,
		"--achievable", "The notes are ready and the footage is shot",
		"--relevant", "It brings followers to the channel",
		"--time-bound", "Before the end of the month",
		"--due", "2026-11-30",
	];
	// The video project: a brief, a script saved four times, media and a timeline with captions, a voice and music.
	const video = cliJson<{ project_id: string }>([
		"project:create", "--name", "Reels 5 filming mistakes", "--kind", "video",
		"--description", "A 45 second vertical Reel for people who film with a phone and want to look professional. Light and sound matter more than the camera.",
		...goal("Publish a 45 second Reel about five filming mistakes", "A thousand views in a week"),
	]).project_id;
	cli(["briefing:answer", "--project", video, "--kind", "short",
		"--answer", "audience=People who film with a phone and want to look professional",
		"--answer", "goal=Publish a 45 second Reel that teaches five filming mistakes",
		"--answer", "platform=instagram-reels", "--answer", "format=vertical-9x16", "--answer", "duration=45", "--answer", "tone=Friendly and direct"]);
	const script = join(work, "script.md");
	const lines = ["# Five filming mistakes", "", "Hook: you are filming against the window, and it shows.", "", "1. Light from the front, not from behind.", "2. Spend one minute on sound before you press record.", "3. Look at the camera, not at yourself.", "4. Keep the phone steady: lean on something.", "5. Cut the first three seconds of dead air.", "", "Close: pick one mistake, fix it today and film again.", ""];
	writeFileSync(script, lines.join("\n"));
	const scriptItem = /pit_[0-9a-z]{10}/.exec(cli(["text:write", "--project", video, "--title", "Script", "--kind", "script", "--text", script]))?.[0] ?? "";
	for (let revision = 1; revision < 4; revision++) {
		writeFileSync(script, `${lines.join("\n")}\nNote of revision ${revision + 1}.\n`);
		cli(["text:write", scriptItem, "--project", video, "--text", script, "--base", String(revision)]);
	}
	cli(["item:add", "--project", video, "--kind", "timeline", "--title", "Reel", "--preset", "9:16", "--fps", "30"]);
	const imported = cliJson<{ assets: { name: string; asset_id: string }[] }>([
		"media:import", ...["take-facing-window.mp4", "broll-microphone.mp4", "alex-intro.mp4", "voice-over.m4a", "calm-track.mp3"].map((f) => join(media, f)),
		"--project", video, "--wait", "--tag", "reels",
	]);
	const asset = (name: string): string => imported.assets.find((a) => a.name === name)?.asset_id ?? "";
	const [v1, captions, voice, music] = [newId("trk"), newId("trk"), newId("trk"), newId("trk")];
	const ops: { op: string; args: Record<string, string> }[] = [
		{ op: "track.add", args: { kind: "visual", name: "Video", id: v1 } },
		{ op: "track.add", args: { kind: "visual", name: "Captions", id: captions } },
		{ op: "track.add", args: { kind: "audio", name: "Voice", id: voice } },
		{ op: "track.add", args: { kind: "audio", name: "Music", id: music } },
	];
	let at = 0;
	for (const [name, file, sourceIn, seconds] of [["Hook", "take-facing-window.mp4", 0, 4], ["Mistake 1", "broll-microphone.mp4", 0, 3], ["Mistake 2", "alex-intro.mp4", 0, 4], ["Mistake 3", "take-facing-window.mp4", 4, 2], ["Final", "broll-microphone.mp4", 3, 1]] as const) {
		const id = newId("itm");
		ops.push({ op: "clip.add", args: { track_id: v1, asset_id: asset(file), start: `${at}s`, source_in: `${sourceIn}s`, duration: `${seconds}s`, id } });
		ops.push({ op: "item.rename", args: { item_id: id, name } });
		at += seconds;
	}
	for (const [text, start, seconds] of [["you are filming against the window", 0, 3.6], ["light from the front", 4, 2.6], ["spend one minute on sound", 7, 3], ["look at the camera", 10.2, 2.4], ["pick one and film again", 12.8, 2.2]] as const) {
		ops.push({ op: "caption.add", args: { track_id: captions, start: `${start}s`, text, duration: `${seconds}s`, id: newId("itm") } });
	}
	ops.push({ op: "audio.add", args: { track_id: voice, asset_id: asset("voice-over.m4a"), start: "0s", duration: "15s" } });
	ops.push({ op: "audio.add", args: { track_id: music, asset_id: asset("calm-track.mp3"), start: "0s", duration: "15s" } });
	const opsFile = join(work, "ops.json");
	writeFileSync(opsFile, JSON.stringify({ operations: ops }));
	cli(["edit", "--project", video, "--ops", opsFile]);

	// The text project: a brief and a blog post, no media (D-134).
	const text = cliJson<{ project_id: string }>([
		"project:create", "--name", "Post natural light at home", "--kind", "text",
		"--description", "A blog post about using natural light at home for people who film with a phone, with three tips and one example.",
		...goal("Publish one blog post on natural light", "Eight hundred words and three tips"),
	]).project_id;
	const brief = join(work, "brief.md");
	writeFileSync(brief, "# Brief\n\n## Audience\n\nPeople who film with a phone at home\n\n## Goal\n\nExplain how to use the window you already have\n\n## Tone\n\nCalm and practical\n");
	cli(["brief:set", "--project", text, "--text", brief]);
	const post = (title: string, slug: string, body: string): string => {
		const file = join(work, `${slug}.md`);
		writeFileSync(file, `---\ntitle: ${title}\nslug: ${slug}\nsummary: ${body.split(". ")[0]}.\ntags: light, phone\nlanguage: en\n---\n${body}\n`);
		return /pit_[0-9a-z]{10}/.exec(cli(["text:write", "--project", text, "--title", `Post: ${title.toLowerCase()}`, "--kind", "post", "--text", file]))?.[0] ?? "";
	};
	const posts = [
		["Natural light at home", "natural-light-at-home", "Stand with your back to the room and let the window light your face. A thin curtain turns a harsh sun into a gentle glow. Put your phone at eye level and film near the window, not in front of a lamp.", "2026-11-27T09:00"],
		["Sound before picture", "sound-before-picture", "Spend one minute on sound before you press record. Turn off the fan, close the door and say one sentence out loud. Listen to it with headphones. A clear voice does more for a short than a sharp picture.", "2026-11-30T10:00"],
		["Window light checklist", "window-light-checklist", "Face the window, never put it behind you. Check the curtain, the time of day and the colour of the walls. Film a five second test and look at it on a big screen before you film the real take.", "2026-11-25T12:00"],
		["Three phone tips", "three-phone-tips", "Lean the phone on a stack of books. Clean the lens with a soft cloth. Switch on the grid lines so the horizon stays level. These three habits make every clip look calmer.", "2026-11-26T18:00"],
	] as const;

	// A folder for the blog posts, and a key for the voice-over: the product keeps both in its vault.
	const blog = join(work, "blog");
	mkdirSync(blog, { recursive: true });
	const secret = join(work, "blog-secret.json");
	writeFileSync(secret, JSON.stringify({ folder: blog.replaceAll("\\", "/"), template: "hugo" }));
	const connection = cliJson<{ id: string }>(["connection:add", "--kind", "blog_files", "--label", "Alex blog folder", "--endpoint", "http://localhost", "--accepts", "text,image,metadata", "--secret-file", secret]).id;
	const key = join(work, "demo-key.txt");
	writeFileSync(key, "sk-demo-not-a-real-key-0000000000000000\n");
	const openai = cliJson<{ id: string }>(["connection:add", "--kind", "openai", "--label", "OpenAI", "--endpoint", "https://api.openai.com", "--accepts", "text,image,audio", "--secret-file", key]).id;
	const price = await api.put("/api/v1/providers/prices", { headers: PERSON_HEADERS, data: { provider: "openai", model: "tts-1", currency: "USD", unit: "mchar", unit_micros: 15_000_000 } });
	if (!price.ok()) throw new Error(`price: ${await price.text()}`);

	const pending: string[] = [];
	for (const [title, slug, body, when] of posts) {
		const item = post(title, slug, body);
		const publication = cliJson<{ id: string }>(["publication:create", "--project", text, "--title", title, "--post", item, "--connection", connection, "--at", when, "--zone", "UTC", "--visibility", "public"]).id;
		const scheduled = cliJson<{ targets: { approval_id: string }[] }>(["publication:schedule", publication]);
		pending.push(scheduled.targets[0].approval_id);
	}
	// A person approves two of the four posts; the other two, and the voice-over below, wait for a yes.
	for (const id of pending.slice(2)) {
		const decided = await api.post(`/api/v1/approvals/${id}/decide`, { headers: PERSON_HEADERS, data: { decision: "approve" } });
		if (!decided.ok()) throw new Error(`approve: ${await decided.text()}`);
	}
	const speech = "Five filming mistakes you can fix today. One: stop filming with your back to the window. Two: spend one minute on sound before you press record. Three: look at the camera, not at yourself. Four: lean the phone on something steady. Five: cut the first three seconds of dead air. Pick one, fix it today and film again.";
	cli(["voice:generate", "--project", video, "--connection", openai, "--model", "tts-1", "--voice", "nova", "--text", speech]);
	// The library: two pictures beside the footage, music and voice-over of the video project.
	cli(["media:import", join(media, "window-light.png"), join(media, "phone-on-books.png"), "--project", video, "--wait", "--tag", "reels"]);

	// The image project and its thumbnail: the picture, a dark band and the title, as layers.
	cli(["project:create", "--name", "Thumbnail filming mistakes", "--kind", "image",
		"--description", "The thumbnail of the Reel on filming mistakes: a warm picture, a dark band and a short title that reads on a phone.",
		...goal("Make one thumbnail that reads at the size of a phone", "One thumbnail ready before the Reel is published")]);
	const thumbnail = cliJson<Record<string, unknown>>(["image:import", "--path", join(media, "thumbnail-background.png"), "--preset", "video_thumbnail", "--name", "Reel thumbnail"]);
	const image = String(thumbnail.image_id ?? thumbnail.id ?? (thumbnail.image as { id?: string } | undefined)?.id ?? "");
	if (!image) throw new Error(`image:import: ${JSON.stringify(thumbnail)}`);
	const imageOps = join(work, "image-ops.json");
	writeFileSync(imageOps, JSON.stringify({ operations: [
		{ op: "layer.add_shape", args: { name: "Band", shape: "rect", x: 0, y: 430, w: 1280, h: 290, fill: "#101218CC" } },
		{ op: "layer.add_text", args: { name: "Title", text: "Five filming mistakes", y: 470, style: { size: 96, font_weight: 800, color: "#FFFFFF" } } },
		{ op: "layer.add_text", args: { name: "Subtitle", text: "Fix them today", y: 600, style: { size: 48, font_weight: 600, color: "#F6CC7C" } } },
	] }));
	cli(["image:edit", image, "--ops", imageOps]);

	// What the agent reads: a skill, a prompt and a memory note, all waiting for a person to review them.
	const skill = join(work, "skill.md");
	writeFileSync(skill, ["# Captions for shorts", "", "1. Keep each caption under six words.", "2. Put the key word of the line last.", "3. Never cover the face: keep captions in the lower third.", "4. Check every caption against the script before you export.", ""].join("\n"));
	cli(["skill:write", "captions-for-shorts", "--description", "Write short, readable captions for a vertical video and check them against the script.", "--body-file", skill]);
	cli(["prompt:write", "five-hooks", "--description", "Five hook lines for the first seconds of a short video.", "--body", "Write five hook lines for a short video about {{topic}}, for {{audience}}. Each line under ten words, no question marks.", "--default", "audience=people who film with a phone", "--tag", "hooks"]);
	cli(["memory:write", "--description", "Alex films at home, by a window that faces west.", "--body", "Plan the takes for the morning, when the window light is soft. After four in the afternoon the light is too warm.", "--tag", "filming"]);

	// The home continues the project touched last: the video, whose path is the one of the design.
	cli(["project:update", video, "--description", "A 45 second vertical Reel for people who film with a phone and want to look professional. Light and sound matter more than the camera, so it shows both."]);
	return { video, text, trailText: text, image };
}

type Ids = { video: string; text: string; image: string };
type Shot = { name: string; path: (ids: Ids) => string; viewport?: { width: number; height: number }; prepare?: (page: Page) => Promise<void>; atTop?: boolean };

const SHOTS: Shot[] = [
	{ name: "dashboard", path: () => "/" },
	{
		name: "project-brief",
		path: () => "/projects",
		prepare: async (page) => {
			await page.getByRole("button", { name: "New project" }).first().click();
			await page.getByText("A video, from the script to the export.").click();
			await page.getByRole("button", { name: "Continue to the briefing" }).click();
			const answers: [string, string][] = [
				["Name", "Reels 5 filming mistakes"],
				["Description", "A 45 second vertical Reel for people who film with a phone and want to look professional. Light and sound matter more than the camera."],
				["Specific", "Publish a 45 second Reel about five filming mistakes"],
				["Measurable", "A thousand views in a week"],
				["Achievable", "The script is ready and the footage is shot"],
				["Relevant", "It brings followers to the channel"],
			];
			for (const [label, value] of answers) await page.getByLabel(label, { exact: true }).fill(value);
		},
	},
	{ name: "approvals", path: () => "/approvals" },
	{ name: "publish-calendar", path: () => "/publish" },
	{ name: "project-trail", path: (ids) => `/projects/${ids.video}` },
	{ name: "text-trail", path: (ids) => `/projects/${ids.text}` },
	{
		name: "video-editor",
		path: (ids) => `/editor/${ids.video}`,
		prepare: async (page) => {
			const skip = page.getByRole("button", { name: "Skip tour" });
			if (await skip.isVisible().catch(() => false)) await skip.click();
			await page.getByText("Mistake 1", { exact: true }).first().click();
		},
	},
	{ name: "home-mobile", path: () => "/", viewport: { width: 390, height: 844 } },
	// The steps of the path of the landing that the screens above do not show (author change after the review).
	{ name: "projects", path: () => "/projects" },
	{
		name: "writing",
		path: () => "/write",
		// Opening the text scrolls the page to it; the picture starts at the top of the page.
		atTop: true,
		prepare: async (page) => {
			await clickIfShown(page, page.getByText("Script", { exact: true }).filter({ visible: true }).first());
			await page.locator("textarea").first().waitFor({ state: "visible", timeout: 15_000 });
		},
	},
	{ name: "video-assets", path: () => "/video-assets" },
	{ name: "visual-assets", path: () => "/visual-assets" },
	{ name: "image-editor", path: (ids) => `/edit-image/${ids.image}`, prepare: (page) => clickIfShown(page, page.getByText("Title", { exact: true }).first()) },
	{ name: "agent-chat", path: (ids) => `/projects/${ids.video}`, prepare: (page) => clickIfShown(page, page.getByRole("button", { name: "Open chat" }).first()) },
	{ name: "skills", path: () => "/skills", prepare: (page) => clickIfShown(page, page.getByText("captions-for-shorts").first()) },
	{ name: "prompts", path: () => "/prompts", prepare: (page) => clickIfShown(page, page.getByText("five-hooks").first()) },
	{ name: "memory", path: () => "/memory", prepare: (page) => clickIfShown(page, page.getByText("Alex films at home", { exact: false }).first()) },
	{ name: "connections", path: () => "/connections" },
];

// A screen that already shows what the step needs has nothing to click; a click that fails would hide a changed screen,
// so the script stops instead of taking a picture of the wrong one.
async function clickIfShown(page: Page, target: import("@playwright/test").Locator): Promise<void> {
	await target.waitFor({ state: "visible", timeout: 15_000 });
	await target.click();
}

async function shoot(browser: import("@playwright/test").Browser, ids: Ids): Promise<void> {
	mkdirSync(shots, { recursive: true });
	for (const theme of ["dark", "light"] as const) {
		for (const shot of SHOTS.filter((x) => only.length === 0 || only.includes(x.name))) {
			const mobile = shot.viewport !== undefined;
			const context = await browser.newContext({ viewport: shot.viewport ?? { width: 1600, height: 1000 }, deviceScaleFactor: mobile ? 1600 / 390 : 1, baseURL: base, timezoneId: "UTC" });
			await context.addInitScript((t: string) => localStorage.setItem("theme", t), theme);
			const login = await context.request.post("/auth/account/login", { headers: JSON_HEADERS, data: { identity: "alex", password: PASSWORD } });
			if (!login.ok()) throw new Error(`login: ${await login.text()}`);
			const page = await context.newPage();
			await page.goto(shot.path(ids), { waitUntil: "networkidle" });
			await shot.prepare?.(page);
			await page.waitForTimeout(1500);
			if (shot.atTop) {
				await page.evaluate(() => {
					(document.activeElement as HTMLElement | null)?.blur();
					for (const el of [document.scrollingElement, ...document.querySelectorAll("*")]) if (el && el.scrollTop > 0) el.scrollTop = 0;
				});
				await page.waitForTimeout(300);
			}
			await page.screenshot({ path: join(shots, `${shot.name}${theme === "light" ? "-light" : ""}.png`) });
			await context.close();
		}
	}
}

function encode(): void {
	for (const png of SHOTS.map((shot) => shot.name).filter((n) => only.length === 0 || only.includes(n)).flatMap((n) => [n, `${n}-light`])) {
		const source = join(shots, `${png}.png`);
		for (const [suffix, width] of [["", 1600], ["-960", 960]] as const) {
			const out = join(publicLanding, `${png}${suffix}`);
			const scale = ["-vf", `scale=${width}:-2:flags=lanczos`];
			execFileSync(ffmpeg, ["-v", "error", "-y", "-i", source, ...scale, "-map_metadata", "-1", "-c:v", "libwebp", "-quality", "80", `${out}.webp`]);
			execFileSync(ffmpeg, ["-v", "error", "-y", "-i", source, ...scale, "-map_metadata", "-1", "-c:v", "libaom-av1", "-crf", "30", "-cpu-used", "6", "-still-picture", "1", `${out}.avif`]);
		}
	}
}

async function main(): Promise<void> {
	checkWork();
	const browser = await chromium.launch();
	try {
		const ids = join(work, "ids.json");
		if (phase === "all" || phase === "seed") {
			rmSync(work, { recursive: true, force: true });
			mkdirSync(work, { recursive: true });
			writeFileSync(join(work, MARKER), "A throwaway folder of site/scripts/capture-landing.ts; the seed phase empties it.\n");
			makeMedia();
			await startServer();
			const context = await browser.newContext({ baseURL: base });
			writeFileSync(ids, JSON.stringify(await seed(context)));
			await context.close();
		}
		if (phase === "all" || phase === "shoot") {
			if (phase === "shoot") await startServerKeep();
			await shoot(browser, JSON.parse(readFileSync(ids, "utf8")) as Ids);
		}
		if (phase === "all" || phase === "encode") encode();
	} finally {
		await browser.close();
		stopServer();
	}
}

// `shoot` alone reuses the workspace a previous `seed` left: the server is started again on it.
async function startServerKeep(): Promise<void> {
	server = spawn(bin, ["serve"], { env, stdio: "ignore" });
	for (let i = 0; i < 120; i++) {
		try {
			if ((await fetch(`${base}/auth/account`)).ok) return;
		} catch {
			// not listening yet
		}
		await new Promise((r) => setTimeout(r, 500));
	}
	throw new Error("the server did not start");
}

await main();
