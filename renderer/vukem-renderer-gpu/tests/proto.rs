//! The pipe protocol of `vukem-render` (SDD-013 section 4.9): the messages, the replies, the limits and
//! the end of input, drawn by the CPU path so no adapter is needed, and the real binary where one exists.

use std::io::{Cursor, Read, Write};
use std::process::{Command, Stdio};

use vukem_renderer::host::Renderer;
use vukem_renderer_gpu::GpuError;
use vukem_renderer_gpu::proto::{Draw, MAX_PAYLOAD, STATUS_PROTOCOL, serve};

const INPUT: &str = r##"{"version":1,"output":{"width":8,"height":8,"scale":1},"plan":{"background":"#102030","layers":[
	{"item":"a","kind":"media","cx":4,"cy":4,"w":4,"h":4,"opacity":1}]}}"##;

struct Cpu;

impl Draw for Cpu {
	fn draw(&self, host: &Renderer, input: &[u8], out: &mut [u8]) -> Result<(), GpuError> {
		host.render(input, out).map_err(GpuError::Render)
	}
}

struct Failing;

impl Draw for Failing {
	fn draw(&self, _: &Renderer, _: &[u8], _: &mut [u8]) -> Result<(), GpuError> {
		Err(GpuError::Device("lost".into()))
	}
}

fn msg(kind: u8, payload: &[u8]) -> Vec<u8> {
	let mut m = vec![kind];
	m.extend((payload.len() as u32).to_le_bytes());
	m.extend(payload);
	m
}

fn init() -> Vec<u8> {
	msg(0, &[0; 8])
}

fn pixels() -> Vec<u8> {
	(0..16)
		.flat_map(|i| [i * 15, 255 - i * 15, 40 + i * 5, 255])
		.collect()
}

fn texture(id: &str, w: u32, h: u32, pix: &[u8]) -> Vec<u8> {
	let mut p = vec![id.len() as u8];
	p.extend(id.as_bytes());
	p.extend(w.to_le_bytes());
	p.extend(h.to_le_bytes());
	p.push(1);
	p.extend(pix);
	msg(1, &p)
}

/// The replies of a run as `(status, payload)`, and how the run ended.
fn run(draw: &impl Draw, requests: &[u8]) -> (Vec<(u8, Vec<u8>)>, std::io::Result<()>) {
	let mut out = Vec::new();
	let end = serve(draw, "test adapter", &mut Cursor::new(requests), &mut out);
	let mut replies = Vec::new();
	let mut r = Cursor::new(out);
	loop {
		let mut head = [0_u8; 5];
		if r.read_exact(&mut head).is_err() {
			break;
		}
		let n = u32::from_le_bytes([head[1], head[2], head[3], head[4]]) as usize;
		let mut body = vec![0_u8; n];
		r.read_exact(&mut body).unwrap();
		replies.push((head[0], body));
	}
	(replies, end)
}

#[test]
fn a_frame_is_what_the_cpu_path_draws() {
	let mut req = init();
	req.extend(texture("a", 4, 4, &pixels()));
	req.extend(msg(4, INPUT.as_bytes()));
	let (replies, end) = run(&Cpu, &req);
	end.unwrap();
	assert_eq!(replies.len(), 3);
	assert_eq!(replies[0], (0, b"test adapter".to_vec()));
	assert_eq!(replies[1], (0, vec![]));

	let mut host = Renderer::new(Default::default());
	host.put_texture("a", 4, 4, &pixels(), true).unwrap();
	let mut want = vec![0_u8; 8 * 8 * 4];
	host.render(INPUT.as_bytes(), &mut want).unwrap();
	assert_eq!(replies[2], (0, want));
}

fn lut_message(id: &str, size: u32, entries: &[u8]) -> Vec<u8> {
	let mut p = vec![id.len() as u8];
	p.extend(id.as_bytes());
	p.extend(size.to_le_bytes());
	p.extend(entries);
	msg(5, &p)
}

/// A table that inverts every colour: two points on a side.
fn inverting_table() -> Vec<u8> {
	(0..8_u16)
		.flat_map(|i| [i & 1, i >> 1 & 1, i >> 2 & 1])
		.flat_map(|v| (65535 - v * 65535).to_le_bytes())
		.collect()
}

#[test]
fn a_lookup_table_is_uploaded_and_drawn_by_the_layer_that_names_it() {
	let input = INPUT.replace(
		"\"opacity\":1",
		"\"opacity\":1,\"lut\":{\"asset\":\"ast_lut\",\"intensity\":1}",
	);
	let mut req = init();
	req.extend(texture("a", 4, 4, &pixels()));
	req.extend(lut_message("ast_lut", 2, &inverting_table()));
	req.extend(msg(4, input.as_bytes()));
	let (replies, end) = run(&Cpu, &req);
	end.unwrap();
	assert_eq!(replies.len(), 4);
	assert_eq!(replies[2], (0, vec![]));

	let mut host = Renderer::new(Default::default());
	host.put_texture("a", 4, 4, &pixels(), true).unwrap();
	host.put_lut("ast_lut", 2, &inverting_table()).unwrap();
	let mut want = vec![0_u8; 8 * 8 * 4];
	host.render(input.as_bytes(), &mut want).unwrap();
	assert_eq!(replies[3], (0, want));

	// Without the upload the layer names a table the host does not have.
	let mut req = init();
	req.extend(texture("a", 4, 4, &pixels()));
	req.extend(msg(4, input.as_bytes()));
	let (replies, _) = run(&Cpu, &req);
	assert_eq!(
		replies[2].0,
		vukem_renderer::error::Reason::MissingTexture as u8
	);

	// A table that is not size^3 x 3 entries is refused and the child goes on.
	let mut req = init();
	req.extend(lut_message("ast_lut", 2, &[0; 10]));
	let (replies, end) = run(&Cpu, &req);
	end.unwrap();
	assert_eq!(
		replies[1].0,
		vukem_renderer::error::Reason::InvalidArgument as u8
	);
}

#[test]
fn a_refused_request_is_answered_and_the_child_goes_on() {
	let mut req = init();
	req.extend(msg(4, INPUT.as_bytes()));
	req.extend(texture("a", 4, 4, &pixels()));
	req.extend(msg(4, INPUT.as_bytes()));
	let (replies, end) = run(&Cpu, &req);
	end.unwrap();
	assert_eq!(
		replies[1],
		(4, b"a".to_vec()),
		"missing_texture names the item"
	);
	assert_eq!(replies[3].0, 0);
}

#[test]
fn a_texture_that_does_not_match_its_size_is_invalid() {
	let mut req = init();
	req.extend(texture("a", 4, 4, &pixels()[..60]));
	let (replies, end) = run(&Cpu, &req);
	end.unwrap();
	assert_eq!(replies[1].0, 7);
}

#[test]
fn the_budget_of_the_init_bounds_the_textures() {
	let mut budget = 32_u32.to_le_bytes().to_vec();
	budget.extend(0_u32.to_le_bytes());
	let mut req = msg(0, &budget);
	req.extend(texture("a", 4, 4, &pixels()));
	let (replies, end) = run(&Cpu, &req);
	end.unwrap();
	assert_eq!(replies[1], (5, b"render_texture_bytes".to_vec()));
}

#[test]
fn a_release_forgets_the_texture() {
	let mut req = init();
	req.extend(texture("a", 4, 4, &pixels()));
	req.extend(msg(3, &[1, b'a']));
	req.extend(msg(4, INPUT.as_bytes()));
	let (replies, _) = run(&Cpu, &req);
	assert_eq!(replies[2], (0, vec![]));
	assert_eq!(replies[3].0, 4);
}

#[test]
fn the_protocol_refuses_what_it_does_not_allow() {
	let cases: [(&str, Vec<u8>); 4] = [
		("before init", msg(4, INPUT.as_bytes())),
		("init twice", [init(), init()].concat()),
		("unknown kind", [init(), msg(200, &[])].concat()),
		("short message", [init(), msg(3, &[5, b'a'])].concat()),
	];
	for (name, req) in cases {
		let (replies, end) = run(&Cpu, &req);
		let last = replies.last().unwrap();
		if name == "short message" {
			assert_eq!(last.0, 7, "{name}");
			end.unwrap();
		} else {
			assert_eq!(last.0, STATUS_PROTOCOL, "{name}");
			assert!(end.is_err(), "{name}");
		}
	}
}

#[test]
fn a_length_over_the_largest_payload_allocates_nothing_and_ends() {
	let mut req = init();
	req.push(1);
	req.extend((MAX_PAYLOAD as u32 + 1).to_le_bytes());
	let (replies, end) = run(&Cpu, &req);
	assert_eq!(replies.len(), 2);
	assert_eq!(replies[1].0, STATUS_PROTOCOL);
	assert!(end.is_err());
}

#[test]
fn a_cut_payload_ends_the_child() {
	let mut req = init();
	let mut cut = texture("a", 4, 4, &pixels());
	cut.truncate(cut.len() - 5);
	req.extend(cut);
	let (replies, end) = run(&Cpu, &req);
	assert_eq!(replies.last().unwrap().0, STATUS_PROTOCOL);
	assert!(end.is_err());
}

#[test]
fn a_failed_device_is_answered_and_ends_the_child() {
	let mut req = init();
	req.extend(msg(4, INPUT.as_bytes()));
	req.extend(msg(4, INPUT.as_bytes()));
	let (replies, end) = run(&Failing, &req);
	assert_eq!(replies.len(), 2);
	assert_eq!(replies[1], (8, b"lost".to_vec()));
	assert!(end.is_err());
}

fn binary() -> Command {
	Command::new(env!("CARGO_BIN_EXE_vukem-render"))
}

#[test]
fn the_binary_serves_the_protocol_on_its_pipes() {
	let probe = binary().arg("--probe").output().unwrap();
	if probe.status.code() == Some(3) {
		eprintln!("SKIP: no graphics adapter on this machine");
		return;
	}
	assert!(probe.status.success(), "{probe:?}");

	let mut req = init();
	req.extend(texture("a", 4, 4, &pixels()));
	req.extend(msg(4, INPUT.as_bytes()));
	let mut child = binary()
		.stdin(Stdio::piped())
		.stdout(Stdio::piped())
		.stderr(Stdio::null())
		.spawn()
		.unwrap();
	child.stdin.take().unwrap().write_all(&req).unwrap();
	let out = child.wait_with_output().unwrap();
	assert!(out.status.success());

	let mut host = Renderer::new(Default::default());
	host.put_texture("a", 4, 4, &pixels(), true).unwrap();
	let mut want = vec![0_u8; 8 * 8 * 4];
	host.render(INPUT.as_bytes(), &mut want).unwrap();
	let frame = &out.stdout[out.stdout.len() - want.len()..];
	assert_eq!(
		frame, want,
		"a one-to-one copy is byte-equal to the CPU path"
	);
}

#[test]
fn the_binary_refuses_an_unknown_argument() {
	let out = binary().arg("--nope").output().unwrap();
	assert_eq!(out.status.code(), Some(2));
	assert!(out.stdout.is_empty());
}
