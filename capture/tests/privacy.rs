//! The privacy rule of the pointer file (SDD-025 section 16.6): no key is ever read. The helper installs no hook of
//! any kind, registers no raw-input device and asks about no key but the three mouse buttons. These tests read
//! the source of the helper and fail if it names a function that could read a key; a function added to read one
//! has to change this list in review.

use std::path::Path;

const FORBIDDEN: [&str; 21] = [
	"SetWindowsHookEx",
	"UnhookWindowsHookEx",
	"GetKeyboardState",
	"GetKeyState",
	"GetKeyNameText",
	"RegisterRawInputDevices",
	"GetRawInput",
	"RegisterHotKey",
	"MapVirtualKey",
	"ToUnicode",
	"ToAscii",
	"keybd_event",
	"SendInput",
	"WH_KEYBOARD",
	"RIDEV_",
	"KeyboardAndMouse",
	"OpenClipboard",
	"GetClipboardData",
	"SetClipboardData",
	"AddClipboardFormatListener",
	"DataExchange",
];

/// Every `.rs` file under `src/`, its subfolders included, by its path inside `src/`.
fn sources() -> Vec<(String, String)> {
	fn walk(dir: &Path, root: &Path, out: &mut Vec<(String, String)>) {
		for entry in std::fs::read_dir(dir).unwrap() {
			let p = entry.unwrap().path();
			if p.is_dir() {
				walk(&p, root, out);
			} else if p.extension().is_some_and(|e| e == "rs") {
				let name = p
					.strip_prefix(root)
					.unwrap()
					.to_string_lossy()
					.replace('\\', "/");
				out.push((name, std::fs::read_to_string(&p).unwrap()));
			}
		}
	}
	let root = Path::new(env!("CARGO_MANIFEST_DIR")).join("src");
	let mut out = Vec::new();
	walk(&root, &root, &mut out);
	out
}

/// The `windows` crate is a direct dependency for Media Foundation only (D-142): a feature that brings the
/// keyboard, the hooks, raw input or the clipboard into reach has to change this test in review.
#[test]
fn the_windows_crate_brings_only_media_foundation_and_com() {
	let toml =
		std::fs::read_to_string(Path::new(env!("CARGO_MANIFEST_DIR")).join("Cargo.toml")).unwrap();
	let line = toml
		.lines()
		.find(|l| l.trim_start().starts_with("windows ="))
		.expect("the windows dependency line");
	assert!(
		line.contains(r#"features = ["Win32_Media_MediaFoundation", "Win32_System_Com"]"#),
		"the features of the windows crate changed: {line}"
	);
}

#[test]
fn no_source_of_the_helper_names_a_function_that_reads_a_key() {
	let files = sources();
	assert!(files.len() >= 10, "the sources were not found");
	for (name, text) in files {
		for f in FORBIDDEN {
			assert!(
				!text.contains(f),
				"{name} names {f}: the helper must not read a key"
			);
		}
	}
}

#[test]
fn the_only_key_state_the_helper_asks_for_is_the_three_mouse_buttons() {
	for (name, text) in sources() {
		let uses = text.matches("GetAsyncKeyState").count();
		if name == "win32.rs" {
			// the declaration and the one call, which goes through the list of the three buttons
			assert_eq!(
				uses, 2,
				"win32.rs asks for a key state in an unexpected place"
			);
			assert!(
				text.contains("MOUSE_BUTTONS.map(|vk|"),
				"the call must walk MOUSE_BUTTONS"
			);
			assert!(text.contains("pub const MOUSE_BUTTONS: [i32; 3] = [0x01, 0x02, 0x04];"));
		} else {
			assert_eq!(uses, 0, "{name} asks for a key state");
		}
	}
}

#[test]
fn a_sample_has_no_room_for_a_key() {
	let text = std::fs::read_to_string(Path::new(env!("CARGO_MANIFEST_DIR")).join("src/events.rs"))
		.unwrap();
	let start = text.find("pub struct Sample").expect("Sample exists");
	let body = &text[start..text[start..].find('}').map(|i| start + i).unwrap()];
	for field in ["x: i32", "y: i32", "inside: bool", "buttons: [bool; 3]"] {
		assert!(body.contains(field), "Sample lost {field}");
	}
	assert_eq!(
		body.matches("pub ").count(),
		5,
		"Sample grew a member: {body}"
	);
}
