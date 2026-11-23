//! Spike of epic 61 task 1. Disposable: never promoted to product code.
//!
//! It opens ONE window of its own (1920x1080, borderless, a synthetic scrolling pattern), captures
//! only that window with Windows Graphics Capture, paces it to a constant frame rate, and pipes raw
//! BGRA frames into an FFmpeg child that writes fragmented MP4. It never touches the desktop,
//! another application, a microphone, system audio or a webcam.

use std::fs::{File, OpenOptions};
use std::io::Write;
use std::os::windows::io::AsRawHandle;
use std::os::windows::process::CommandExt;
use std::path::PathBuf;
use std::process::{Command, Stdio};
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::thread;
use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

use windows::Win32::Foundation::{COLORREF, FILETIME, HANDLE, HWND, LPARAM, LRESULT, RECT, WPARAM};
use windows::Win32::Graphics::Dwm::DwmFlush;
use windows::Win32::Graphics::Gdi::{
	BitBlt, CreateCompatibleBitmap, CreateCompatibleDC, CreateSolidBrush, FillRect, GetDC, HBRUSH, ReleaseDC, SRCCOPY,
	SelectObject, SetBkColor, SetTextColor, TextOutW,
};
use windows::Win32::System::LibraryLoader::GetModuleHandleW;
use windows::Win32::System::Threading::{
	GetCurrentProcess, GetCurrentThread, GetProcessTimes, GetSystemTimes, GetThreadTimes, SetThreadPriority,
	THREAD_PRIORITY_HIGHEST,
};
use windows::Win32::UI::HiDpi::{DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2, SetProcessDpiAwarenessContext};
use windows::Win32::UI::WindowsAndMessaging::{
	CreateWindowExW, DefWindowProcW, DestroyWindow, DispatchMessageW, MSG, PM_REMOVE, PeekMessageW, RegisterClassW,
	SW_SHOWNOACTIVATE, ShowWindow, TranslateMessage, WNDCLASSW, WS_EX_NOACTIVATE, WS_EX_TOOLWINDOW, WS_POPUP,
};
use windows::core::{PCWSTR, w};

use windows_capture::capture::{Context, GraphicsCaptureApiHandler};
use windows_capture::frame::Frame;
use windows_capture::graphics_capture_api::InternalCaptureControl;
use windows_capture::settings::{
	ColorFormat, CursorCaptureSettings, DirtyRegionSettings, DrawBorderSettings, MinimumUpdateIntervalSettings,
	SecondaryWindowSettings, Settings,
};
use windows_capture::window::Window;

const WIDTH: u32 = 1920;
const HEIGHT: u32 = 1080;
static PAINTED: AtomicU64 = AtomicU64::new(0);
const FRAME_BYTES: usize = (WIDTH * HEIGHT * 4) as usize;

type BoxErr = Box<dyn std::error::Error + Send + Sync>;

struct Args {
	out: PathBuf,
	seconds: u64,
	fps: u32,
	ffmpeg: PathBuf,
	encoder: String,
	bitrate: String,
	log: PathBuf,
	pos: (i32, i32),
	flush_packets: Option<String>,
	/// Extra FFmpeg output options for experiments, split on spaces.
	extra: String,
}

fn parse_args() -> Result<Args, String> {
	let mut a = Args {
		out: PathBuf::from("capture.mp4"),
		seconds: 10,
		fps: 60,
		ffmpeg: PathBuf::from("ffmpeg.exe"),
		encoder: "h264_nvenc".into(),
		bitrate: "12M".into(),
		log: PathBuf::from("spike.log"),
		pos: (0, 0),
		flush_packets: None,
		extra: String::new(),
	};
	let mut it = std::env::args().skip(1);
	while let Some(k) = it.next() {
		let v = it.next().ok_or(format!("missing value for {k}"))?;
		match k.as_str() {
			"--out" => a.out = v.into(),
			"--seconds" => a.seconds = v.parse().map_err(|e| format!("{e}"))?,
			"--fps" => a.fps = v.parse().map_err(|e| format!("{e}"))?,
			"--ffmpeg" => a.ffmpeg = v.into(),
			"--encoder" => a.encoder = v,
			"--bitrate" => a.bitrate = v,
			"--flush-packets" => a.flush_packets = Some(v),
			"--extra" => a.extra = v,
			"--log" => a.log = v.into(),
			"--pos" => {
				let (x, y) = v.split_once(',').ok_or("--pos x,y")?;
				a.pos = (x.parse().map_err(|e| format!("{e}"))?, y.parse().map_err(|e| format!("{e}"))?);
			}
			_ => return Err(format!("unknown flag {k}")),
		}
	}
	Ok(a)
}

fn ft(f: &FILETIME) -> u64 {
	(u64::from(f.dwHighDateTime) << 32) | u64::from(f.dwLowDateTime)
}

/// CPU time (kernel + user) of a process handle, in 100 ns units.
fn process_cpu(h: HANDLE) -> u64 {
	let (mut c, mut e, mut k, mut u) = (FILETIME::default(), FILETIME::default(), FILETIME::default(), FILETIME::default());
	// SAFETY: the pointers are valid for the call and the handle belongs to this process or its child.
	if unsafe { GetProcessTimes(h, &mut c, &mut e, &mut k, &mut u) }.is_ok() {
		ft(&k) + ft(&u)
	} else {
		0
	}
}

fn thread_cpu() -> u64 {
	let (mut c, mut e, mut k, mut u) = (FILETIME::default(), FILETIME::default(), FILETIME::default(), FILETIME::default());
	// SAFETY: as above, for the current thread.
	if unsafe { GetThreadTimes(GetCurrentThread(), &mut c, &mut e, &mut k, &mut u) }.is_ok() {
		ft(&k) + ft(&u)
	} else {
		0
	}
}

/// (idle, total) system time in 100 ns units, for the load context of the whole machine.
fn system_times() -> (u64, u64) {
	let (mut i, mut k, mut u) = (FILETIME::default(), FILETIME::default(), FILETIME::default());
	// SAFETY: the pointers are valid for the call.
	if unsafe { GetSystemTimes(Some(&mut i), Some(&mut k), Some(&mut u)) }.is_ok() {
		(ft(&i), ft(&k) + ft(&u))
	} else {
		(0, 0)
	}
}

unsafe extern "system" fn wndproc(hwnd: HWND, msg: u32, w: WPARAM, l: LPARAM) -> LRESULT {
	// SAFETY: forwards the arguments the system gave us.
	unsafe { DefWindowProcW(hwnd, msg, w, l) }
}

/// The synthetic source: a window of ours with a scrolling barber-pole pattern and a clock.
/// Returns the thread CPU time spent painting, so it can be subtracted from the capture cost.
fn window_thread(title: String, pos: (i32, i32), ready: std::sync::mpsc::Sender<Result<(), String>>, stop: Arc<AtomicBool>) -> u64 {
	let wtitle: Vec<u16> = title.encode_utf16().chain(std::iter::once(0)).collect();
	// SAFETY: plain Win32 calls on a window this thread creates and owns.
	unsafe {
		let module = match GetModuleHandleW(None) {
			Ok(m) => m,
			Err(e) => {
				let _ = ready.send(Err(format!("GetModuleHandleW: {e}")));
				return 0;
			}
		};
		let class = w!("StuartyCaptureSpikeWindow");
		let wc = WNDCLASSW { lpfnWndProc: Some(wndproc), hInstance: module.into(), lpszClassName: class, ..Default::default() };
		RegisterClassW(&wc);
		let hwnd = match CreateWindowExW(
			WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE,
			class,
			PCWSTR(wtitle.as_ptr()),
			WS_POPUP,
			pos.0,
			pos.1,
			WIDTH as i32,
			HEIGHT as i32,
			None,
			None,
			Some(module.into()),
			None,
		) {
			Ok(h) => h,
			Err(e) => {
				let _ = ready.send(Err(format!("CreateWindowExW: {e}")));
				return 0;
			}
		};
		let _ = ShowWindow(hwnd, SW_SHOWNOACTIVATE);
		let _ = ready.send(Ok(()));

		let palette: Vec<HBRUSH> = (0..24u32)
			.map(|i| {
				let h = i as f32 / 24.0;
				let (r, g, b) = hsv(h);
				CreateSolidBrush(COLORREF(u32::from(r) | (u32::from(g) << 8) | (u32::from(b) << 16)))
			})
			.collect();
		let black = CreateSolidBrush(COLORREF(0));
		// The pattern is drawn once into an off-screen bitmap one period wider than the window, then
		// one BitBlt per frame scrolls it: a single GDI operation keeps the source cheap and its
		// cost out of the capture numbers.
		const STRIPE: i32 = 30;
		const PERIOD: i32 = STRIPE * 24;
		let hdc0 = GetDC(Some(hwnd));
		let memdc = CreateCompatibleDC(Some(hdc0));
		let bmp = CreateCompatibleBitmap(hdc0, WIDTH as i32 + PERIOD, HEIGHT as i32);
		SelectObject(memdc, bmp.into());
		ReleaseDC(Some(hwnd), hdc0);
		for i in 0..=((WIDTH as i32 + PERIOD) / STRIPE) {
			let r = RECT { left: i * STRIPE, top: 0, right: (i + 1) * STRIPE, bottom: HEIGHT as i32 };
			FillRect(memdc, &r, palette[(i % 24) as usize]);
		}
		let t0 = Instant::now();
		let mut msg = MSG::default();
		while !stop.load(Ordering::Relaxed) {
			while PeekMessageW(&mut msg, None, 0, 0, PM_REMOVE).as_bool() {
				let _ = TranslateMessage(&msg);
				DispatchMessageW(&msg);
			}
			let ms = t0.elapsed().as_millis() as i64;
			let off = ((ms * 6 / 10) as i32) % PERIOD; // 600 px/s
			let hdc = GetDC(Some(hwnd));
			let _ = BitBlt(hdc, 0, 0, WIDTH as i32, HEIGHT as i32, Some(memdc), off, 0, SRCCOPY);
			let box_r = RECT { left: 40, top: 40, right: 560, bottom: 110 };
			FillRect(hdc, &box_r, black);
			SetBkColor(hdc, COLORREF(0));
			SetTextColor(hdc, COLORREF(0x00FFFFFF));
			let text: Vec<u16> = format!("synthetic t={ms} ms").encode_utf16().collect();
			let _ = TextOutW(hdc, 60, 60, &text);
			ReleaseDC(Some(hwnd), hdc);
			PAINTED.fetch_add(1, Ordering::Relaxed);
			let _ = DwmFlush();
		}
		let _ = DestroyWindow(hwnd);
	}
	thread_cpu()
}

fn hsv(h: f32) -> (u8, u8, u8) {
	let i = (h * 6.0).floor();
	let f = h * 6.0 - i;
	let (p, q, t) = (0.0, 1.0 - f, f);
	let (r, g, b) = match (i as i32) % 6 {
		0 => (1.0, t, p),
		1 => (q, 1.0, p),
		2 => (p, 1.0, t),
		3 => (p, q, 1.0),
		4 => (t, p, 1.0),
		_ => (1.0, p, q),
	};
	((r * 255.0) as u8, (g * 255.0) as u8, (b * 255.0) as u8)
}

/// Latest-frame mailbox between the capture callback and the pacing writer: the callback replaces
/// whatever is waiting, the writer takes one frame per slot of the constant-rate grid. A frame that
/// is replaced before it is taken was surplus (the source runs faster than the output rate).
#[derive(Default)]
struct Mailbox {
	latest: Mutex<Option<Vec<u8>>>,
	spare: Mutex<Vec<Vec<u8>>>,
}

#[derive(Default)]
struct Counters {
	arrived: AtomicU64,
	/// Frames replaced in the mailbox before the writer took them (surplus, not a loss).
	surplus: AtomicU64,
	/// Slots of the 60 fps grid filled with a new frame.
	written_unique: AtomicU64,
	/// Slots of the grid with no new frame since the previous slot: the previous one is repeated.
	/// This is what the spike calls a dropped frame.
	written_dup: AtomicU64,
	max_write_us: AtomicU64,
	writes_over_100ms: AtomicU64,
	writes_over_1s: AtomicU64,
	copy_us_total: AtomicU64,
	copied: AtomicU64,
	/// Gaps between frame arrivals: <10, <20, <30, <50, <100, >=100 ms.
	gap_buckets: [AtomicU64; 6],
}

struct Config {
	mailbox: Arc<Mailbox>,
	counters: Arc<Counters>,
}

struct Capture {
	cfg: Config,
	last_arrival: Option<Instant>,
}

impl GraphicsCaptureApiHandler for Capture {
	type Flags = Config;
	type Error = BoxErr;

	fn new(ctx: Context<Self::Flags>) -> Result<Self, Self::Error> {
		Ok(Self { cfg: ctx.flags, last_arrival: None })
	}

	fn on_frame_arrived(&mut self, frame: &mut Frame, _c: InternalCaptureControl) -> Result<(), Self::Error> {
		let c = &self.cfg.counters;
		c.arrived.fetch_add(1, Ordering::Relaxed);
		let now = Instant::now();
		if let Some(prev) = self.last_arrival.replace(now) {
			let gap_ms = now.duration_since(prev).as_secs_f64() * 1000.0;
			let bucket = match gap_ms {
				g if g < 10.0 => 0,
				g if g < 20.0 => 1,
				g if g < 30.0 => 2,
				g if g < 50.0 => 3,
				g if g < 100.0 => 4,
				_ => 5,
			};
			c.gap_buckets[bucket].fetch_add(1, Ordering::Relaxed);
		}
		if frame.width() != WIDTH || frame.height() != HEIGHT {
			return Err(format!("unexpected frame size {}x{}", frame.width(), frame.height()).into());
		}
		let t = Instant::now();
		let mut data = self.cfg.mailbox.spare.lock().map_err(|_| "poisoned")?.pop().unwrap_or_else(|| Vec::with_capacity(FRAME_BYTES));
		let mut scratch = Vec::new();
		let fb = frame.buffer()?;
		let px = fb.as_nopadding_buffer(&mut scratch);
		data.clear();
		data.extend_from_slice(px);
		c.copy_us_total.fetch_add(t.elapsed().as_micros() as u64, Ordering::Relaxed);
		c.copied.fetch_add(1, Ordering::Relaxed);
		let old = self.cfg.mailbox.latest.lock().map_err(|_| "poisoned")?.replace(data);
		if let Some(old) = old {
			c.surplus.fetch_add(1, Ordering::Relaxed);
			self.cfg.mailbox.spare.lock().map_err(|_| "poisoned")?.push(old);
		}
		Ok(())
	}
}

/// Writes one frame per slot of an absolute constant-rate grid into FFmpeg. A slot with no new
/// frame repeats the previous one, so the file keeps its real length, and is counted as dropped.
/// If the pipe blocks (the encoder stalls), the grid keeps its time: the missed slots are written
/// as soon as the pipe frees, and counted the same way.
fn writer_thread(mut stdin: std::process::ChildStdin, mb: Arc<Mailbox>, stop: Arc<AtomicBool>, fps: u32, c: Arc<Counters>) {
	let slot = Duration::from_secs_f64(1.0 / f64::from(fps));
	// SAFETY: raises the priority of this thread only.
	unsafe {
		let _ = SetThreadPriority(GetCurrentThread(), THREAD_PRIORITY_HIGHEST);
	}
	// The grid starts with the first frame the capture delivers.
	let (t0, first) = loop {
		if stop.load(Ordering::Relaxed) {
			return;
		}
		if let Some(f) = mb.latest.lock().unwrap().take() {
			break (Instant::now(), f);
		}
		thread::sleep(Duration::from_millis(1));
	};
	let mut prev = Some(first);
	let mut k: u32 = 0;
	let mut fresh = true;
	while !stop.load(Ordering::Relaxed) {
		let due = t0 + slot * k;
		if let Some(wait) = due.checked_duration_since(Instant::now()) {
			thread::sleep(wait);
		}
		if k > 0 {
			if let Some(f) = mb.latest.lock().unwrap().take() {
				if let Some(old) = prev.replace(f) {
					mb.spare.lock().unwrap().push(old);
				}
				fresh = true;
			} else {
				fresh = false;
			}
		}
		let t = Instant::now();
		if stdin.write_all(prev.as_ref().expect("first frame is set")).is_err() {
			break;
		}
		let us = t.elapsed().as_micros() as u64;
		c.max_write_us.fetch_max(us, Ordering::Relaxed);
		if us > 100_000 {
			c.writes_over_100ms.fetch_add(1, Ordering::Relaxed);
		}
		if us > 1_000_000 {
			c.writes_over_1s.fetch_add(1, Ordering::Relaxed);
		}
		if fresh { c.written_unique.fetch_add(1, Ordering::Relaxed); } else { c.written_dup.fetch_add(1, Ordering::Relaxed); }
		k += 1;
	}
}

fn log_line(f: &mut File, s: &str) {
	println!("{s}");
	let _ = writeln!(f, "{s}");
	let _ = f.flush();
}

fn main() {
	if let Err(e) = run() {
		eprintln!("spike error: {e}");
		std::process::exit(1);
	}
}

fn run() -> Result<(), BoxErr> {
	let args = parse_args()?;
	// SAFETY: called once at start-up, before any window exists.
	unsafe {
		let _ = SetProcessDpiAwarenessContext(DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2);
	}
	let mut log = OpenOptions::new().create(true).append(true).open(&args.log)?;
	let nonce = SystemTime::now().duration_since(UNIX_EPOCH)?.as_nanos();
	let title = format!("stuarty-capture-spike-{}-{nonce}", std::process::id());

	let stop_window = Arc::new(AtomicBool::new(false));
	let (rtx, rrx) = std::sync::mpsc::channel();
	let win = {
		let (t, s, p) = (title.clone(), stop_window.clone(), args.pos);
		thread::spawn(move || window_thread(t, p, rtx, s))
	};
	rrx.recv()?.map_err(BoxErr::from)?;
	thread::sleep(Duration::from_millis(500));
	let window = Window::from_name(&title)?;

	let fps = args.fps;
	let mut ffargs: Vec<String> = [
		"-hide_banner", "-loglevel", "info", "-y", "-f", "rawvideo", "-pix_fmt", "bgra", "-video_size",
		&format!("{WIDTH}x{HEIGHT}"), "-framerate", &fps.to_string(), "-i", "pipe:0",
	]
	.iter()
	.map(|s| (*s).to_string())
	.collect();
	// `--encoder null` measures the capture and the pipe alone, with no encoder behind them.
	if args.encoder == "null" {
		ffargs.extend(["-f", "null", "-"].map(String::from));
	} else {
		ffargs.extend(
			[
				"-c:v", &args.encoder, "-b:v", &args.bitrate, "-g", &fps.to_string(), "-movflags",
				"+frag_keyframe+empty_moov+default_base_moof", "-f", "mp4",
			]
			.map(String::from),
		);
		// Measured (see REPORT.md): without these, killing FFmpeg loses about 4 s of the tail to the
		// encoder's own latency and the muxer's buffering; with them, under 0.5 s.
		let extra = if args.extra.is_empty() && args.encoder == "h264_nvenc" {
			"-delay 0 -rc-lookahead 0 -bf 0"
		} else {
			args.extra.as_str()
		};
		ffargs.extend(extra.split_whitespace().map(String::from));
		ffargs.extend(["-flush_packets".to_string(), args.flush_packets.clone().unwrap_or_else(|| "1".into())]);
		ffargs.push(args.out.to_string_lossy().into_owned());
	}
	let ff_err = File::create(args.out.with_extension("ffmpeg.log"))?;
	let mut child = Command::new(&args.ffmpeg)
		.args(&ffargs)
		.stdin(Stdio::piped())
		.stdout(Stdio::null())
		.stderr(Stdio::from(ff_err))
		.creation_flags(0x0800_0000)
		.spawn()?;
	let stdin = child.stdin.take().ok_or("no ffmpeg stdin")?;
	log_line(&mut log, &format!("start pid={} ffmpeg_pid={} encoder={} fps={fps} seconds={}", std::process::id(), child.id(), args.encoder, args.seconds));

	let counters = Arc::new(Counters::default());
	let mailbox = Arc::new(Mailbox::default());
	let stop_writer = Arc::new(AtomicBool::new(false));
	let writer = {
		let (c, mb, s) = (counters.clone(), mailbox.clone(), stop_writer.clone());
		thread::spawn(move || writer_thread(stdin, mb, s, fps, c))
	};

	let settings = Settings::new(
		window,
		CursorCaptureSettings::WithoutCursor,
		DrawBorderSettings::WithoutBorder,
		SecondaryWindowSettings::Exclude,
		// Measured: with `Default` this Windows 11 build delivers about 42 frames/s for a 74 Hz
		// window; any explicit interval lifts it to the compositor rate. The constant rate is made by
		// the writer's grid, so the interval only has to stay under one frame.
		MinimumUpdateIntervalSettings::Custom(Duration::from_millis(8)),
		DirtyRegionSettings::Default,
		ColorFormat::Bgra8,
		Config { mailbox: mailbox.clone(), counters: counters.clone() },
	);
	let started = Instant::now();
	let (idle0, total0) = system_times();
	let proc0 = process_cpu(unsafe { GetCurrentProcess() });
	let control = Capture::start_free_threaded(settings).map_err(|e| format!("start capture: {e}"))?;

	let (mut last_idle, mut last_total) = (idle0, total0);
	let mut next_tick = Duration::from_secs(5);
	while started.elapsed() < Duration::from_secs(args.seconds) {
		thread::sleep(Duration::from_millis(200));
		if started.elapsed() >= next_tick {
			next_tick += Duration::from_secs(5);
			let (i, t) = system_times();
			let load = 100.0 * (1.0 - (i - last_idle) as f64 / (t - last_total).max(1) as f64);
			(last_idle, last_total) = (i, t);
			log_line(&mut log, &format!(
				"t={:.1}s painted={} arrived={} surplus={} unique={} dup={} maxwrite_ms={:.1} sysload={load:.0}%",
				started.elapsed().as_secs_f64(),
				PAINTED.load(Ordering::Relaxed),
				counters.arrived.load(Ordering::Relaxed),
				counters.surplus.load(Ordering::Relaxed),
				counters.written_unique.load(Ordering::Relaxed),
				counters.written_dup.load(Ordering::Relaxed),
				counters.max_write_us.load(Ordering::Relaxed) as f64 / 1000.0,
			));
		}
	}
	let elapsed = started.elapsed();
	let proc_cpu = process_cpu(unsafe { GetCurrentProcess() }) - proc0;
	let (i, t) = system_times();
	let sys_load = 100.0 * (1.0 - (i - idle0) as f64 / (t - total0).max(1) as f64);
	control.stop().map_err(|e| format!("stop capture: {e}"))?;
	stop_writer.store(true, Ordering::Relaxed);
	writer.join().map_err(|_| "writer panicked")?;
	let status = child.wait()?;
	let ff_cpu = process_cpu(HANDLE(child.as_raw_handle()));
	stop_window.store(true, Ordering::Relaxed);
	let paint_cpu = win.join().map_err(|_| "window thread panicked")?;

	let c = &counters;
	let unique = c.written_unique.load(Ordering::Relaxed);
	let dup = c.written_dup.load(Ordering::Relaxed);
	let secs = elapsed.as_secs_f64();
	let expected = (secs * f64::from(fps)).floor();
	let cores = thread::available_parallelism().map(|n| n.get()).unwrap_or(1) as f64;
	let pct = |x: u64| 100.0 * x as f64 / 1e7 / secs;
	log_line(&mut log, &format!("ffmpeg_exit={status}"));
	log_line(&mut log, &format!("elapsed_s={secs:.2} expected_slots={expected} unique={unique} dup={dup} surplus={} arrived={} writes_over_100ms={} writes_over_1s={}",
		c.surplus.load(Ordering::Relaxed), c.arrived.load(Ordering::Relaxed),
		c.writes_over_100ms.load(Ordering::Relaxed), c.writes_over_1s.load(Ordering::Relaxed)));
	log_line(&mut log, &format!("dropped_pct_of_slots={:.3} (dup / (unique+dup)); missing_vs_expected_pct={:.3}",
		100.0 * dup as f64 / (unique + dup).max(1) as f64,
		100.0 * (expected - (unique + dup) as f64).max(0.0) / expected.max(1.0)));
	log_line(&mut log, &format!("cpu_pct_of_one_core: helper_total={:.1} helper_paint_thread={:.1} helper_capture_side={:.1} ffmpeg={:.1} (machine has {cores} logical cores)",
		pct(proc_cpu), pct(paint_cpu), pct(proc_cpu.saturating_sub(paint_cpu)), pct(ff_cpu)));
	let g: Vec<u64> = c.gap_buckets.iter().map(|b| b.load(Ordering::Relaxed)).collect();
	log_line(&mut log, &format!("arrival_gaps_ms <10:{} <20:{} <30:{} <50:{} <100:{} >=100:{}", g[0], g[1], g[2], g[3], g[4], g[5]));
	log_line(&mut log, &format!("copy_ms_per_frame={:.2} max_pipe_write_ms={:.1} system_cpu_load_pct_during_run={sys_load:.0}",
		c.copy_us_total.load(Ordering::Relaxed) as f64 / 1000.0 / c.copied.load(Ordering::Relaxed).max(1) as f64,
		c.max_write_us.load(Ordering::Relaxed) as f64 / 1000.0));
	Ok(())
}
