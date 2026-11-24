//! The two commands: `probe` loads the runtime, `faces` runs YuNet over the frames of standard input.

use std::io::{Read, Write};
use std::time::Instant;

use ort::ep::{self, ExecutionProvider};
use ort::session::Session;
use ort::session::builder::{GraphOptimizationLevel, SessionBuilder};
use ort::value::Tensor;

use crate::protocol::{Args, HelperError, PROTOCOL, Provider, json_string};
use crate::yunet::{self, Face, Head};

const NMS_IOU: f32 = 0.3;
const MAX_FACES: usize = 16;

fn ort_err(code: &'static str, what: &str, e: &impl std::fmt::Display) -> HelperError {
	// Only the text of the runtime, never a path of this machine.
	HelperError::new(code, format!("{what}: {e}"))
}

fn load_environment(runtime: &str) -> Result<ort::environment::Environment, HelperError> {
	ort::init_from(runtime)
		.map_err(|e| {
			HelperError::new(
				"runtime_load",
				format!("the ONNX Runtime library could not be loaded: {e}"),
			)
		})?
		.with_name("stuarty-aihelper")
		.build()
		.map_err(|e| {
			ort_err(
				"runtime_load",
				"the ONNX Runtime environment could not be made",
				&e,
			)
		})
}

pub fn probe(args: &Args, out: &mut impl Write) -> Result<(), HelperError> {
	let _env = load_environment(&args.runtime)?;
	let directml = ep::DirectML::is_available().unwrap_or(false);
	writeln!(
		out,
		r#"{{"type":"probe","protocol":{PROTOCOL},"build":{},"directml":{directml}}}"#,
		json_string(ort::info())
	)
	.map_err(io_err)
}

fn io_err(e: std::io::Error) -> HelperError {
	HelperError::new("io", e.to_string())
}

struct Opened {
	session: Session,
	provider: &'static str,
}

fn builder(env: &ort::environment::Environment, args: &Args) -> Result<SessionBuilder, String> {
	let mut b = Session::builder(env)
		.map_err(|e| e.to_string())?
		.with_optimization_level(GraphOptimizationLevel::Level3)
		.map_err(|e| e.to_string())?;
	if args.threads > 0 {
		b = b
			.with_intra_threads(args.threads)
			.map_err(|e| e.to_string())?;
	}
	Ok(b)
}

/// The DirectML recommendations of the runtime: a fixed memory pattern off and no parallel execution.
fn open_directml(env: &ort::environment::Environment, args: &Args) -> Result<Session, String> {
	let mut b = builder(env, args)?
		.with_execution_providers([ep::DirectML::default().build().error_on_failure()])
		.map_err(|e| e.to_string())?
		.with_memory_pattern(false)
		.map_err(|e| e.to_string())?
		.with_parallel_execution(false)
		.map_err(|e| e.to_string())?;
	b.commit_from_file(&args.model).map_err(|e| e.to_string())
}

fn open(env: &ort::environment::Environment, args: &Args) -> Result<Opened, HelperError> {
	if args.provider != Provider::Cpu {
		match open_directml(env, args) {
			Ok(session) => {
				return Ok(Opened {
					session,
					provider: "directml",
				});
			}
			Err(e) if args.provider == Provider::DirectMl => {
				return Err(ort_err(
					"provider_unavailable",
					"DirectML could not be used",
					&e,
				));
			}
			Err(_) => {}
		}
	}

	let mut b = builder(env, args)
		.map_err(|e| ort_err("session", "the session could not be started", &e))?;
	let session = b
		.commit_from_file(&args.model)
		.map_err(|e| ort_err("model_load", "the model could not be loaded", &e))?;
	Ok(Opened {
		session,
		provider: "cpu",
	})
}

/// Reads one frame; `Ok(false)` is a clean end of input between frames.
fn read_frame(input: &mut impl Read, buf: &mut [u8]) -> Result<bool, HelperError> {
	let mut filled = 0;
	while filled < buf.len() {
		let n = input.read(&mut buf[filled..]).map_err(io_err)?;
		if n == 0 {
			return if filled == 0 {
				Ok(false)
			} else {
				Err(HelperError::new(
					"short_frame",
					"the input ended inside a frame",
				))
			};
		}
		filled += n;
	}
	Ok(true)
}

fn write_faces(out: &mut impl Write, index: u64, faces: &[Face]) -> Result<(), HelperError> {
	let mut line = format!(r#"{{"type":"frame","index":{index},"faces":["#);
	for (i, f) in faces.iter().enumerate() {
		if i > 0 {
			line.push(',');
		}
		line.push_str(&format!(
			r#"{{"x":{:.1},"y":{:.1},"w":{:.1},"h":{:.1},"score":{:.3}}}"#,
			f.x, f.y, f.w, f.h, f.score
		));
	}
	line.push_str("]}");
	writeln!(out, "{line}").map_err(io_err)
}

pub fn faces(args: &Args, input: &mut impl Read, out: &mut impl Write) -> Result<(), HelperError> {
	let env = load_environment(&args.runtime)?;
	let Opened {
		mut session,
		provider,
	} = open(&env, args)?;
	let input_name = session
		.inputs()
		.first()
		.map(|o| o.name().to_string())
		.ok_or_else(|| HelperError::new("model", "the model has no input"))?;
	writeln!(
		out,
		r#"{{"type":"ready","protocol":{PROTOCOL},"provider":"{provider}","width":{},"height":{}}}"#,
		args.width, args.height
	)
	.map_err(io_err)?;
	out.flush().map_err(io_err)?;

	let (w, h) = (args.width, args.height);
	let min_score = args.min_score_milli as f32 / 1000.0;
	let mut frame = vec![0_u8; w * h * 3];
	let (mut count, mut infer_us) = (0_u64, 0_u128);
	while read_frame(input, &mut frame)? {
		let started = Instant::now();
		let tensor = Tensor::from_array(([1_usize, 3, h, w], yunet::to_nchw_bgr(&frame, w, h)))
			.map_err(|e| ort_err("inference", "the frame could not be given to the model", &e))?;
		let outputs = session
			.run(ort::inputs![input_name.as_str() => tensor])
			.map_err(|e| ort_err("inference", "the model failed on a frame", &e))?;
		let get = |name: &str| -> Result<&[f32], HelperError> {
			let value = outputs.get(name).ok_or_else(|| {
				HelperError::new("model", format!("the model has no output {name}"))
			})?;
			value
				.try_extract_tensor::<f32>()
				.map(|(_, data)| data)
				.map_err(|e| ort_err("model", "an output is not a float tensor", &e))
		};
		let mut heads = Vec::with_capacity(3);
		for stride in yunet::STRIDES {
			heads.push(Head {
				stride,
				cls: get(&format!("cls_{stride}"))?,
				obj: get(&format!("obj_{stride}"))?,
				bbox: get(&format!("bbox_{stride}"))?,
			});
		}
		let found = yunet::decode(&heads, w, h, min_score, NMS_IOU, MAX_FACES);
		infer_us += started.elapsed().as_micros();
		write_faces(out, count, &found)?;
		count += 1;
	}
	writeln!(
		out,
		r#"{{"type":"done","frames":{count},"infer_ms":{}}}"#,
		infer_us / 1000
	)
	.map_err(io_err)
}
