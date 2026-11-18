//! The device, the pipeline and one render on the GPU.

use core::fmt;
use std::mem::size_of;

use vukem_renderer::blend::BlendMode;
use vukem_renderer::compose::{Layer, Prepared, Target, copy_origin, frame_fill, prepare_layer};
use vukem_renderer::effects::Direction;
use vukem_renderer::error::{Error, Reason};
use vukem_renderer::host::Renderer;
use vukem_renderer::mask::MaskGeometry;
use vukem_renderer::plan::decode;
use wgpu::util::{BufferInitDescriptor, DeviceExt, TextureDataOrder};

use crate::block::block_on;
use crate::quad::quad_for;

const FORMAT: wgpu::TextureFormat = wgpu::TextureFormat::Rgba8Unorm;
/// Bytes of one vertex: a position and a texel position, two `f32` each.
const VERTEX_BYTES: u64 = 4 * size_of::<f32>() as u64;
/// Row pitch of a buffer copy.
const ROW_ALIGN: usize = wgpu::COPY_BYTES_PER_ROW_ALIGNMENT as usize;

/// Why the GPU path could not draw.
#[derive(Debug)]
pub enum GpuError {
	/// No adapter of the requested kind exists on this machine.
	NoAdapter,
	/// The device or a buffer failed, with the message of `wgpu`.
	Device(String),
	/// The render input or the stored pixels are not drawable (the CPU path fails the same way).
	Render(Error),
}

impl fmt::Display for GpuError {
	fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
		match self {
			Self::NoAdapter => f.write_str("no graphics adapter"),
			Self::Device(m) => write!(f, "gpu: {m}"),
			Self::Render(e) => e.fmt(f),
		}
	}
}

impl std::error::Error for GpuError {}

impl From<Error> for GpuError {
	fn from(e: Error) -> Self {
		Self::Render(e)
	}
}

fn device_err(e: impl fmt::Display) -> GpuError {
	GpuError::Device(e.to_string())
}

/// The kind of adapter to ask for.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Adapter {
	/// The high-performance adapter the system offers.
	Any,
	/// The software adapter (WARP on Direct3D 12, llvmpipe on Vulkan), the one a CI machine without
	/// a GPU has.
	Software,
}

/// A device with the pipeline of the GPU path. Rendering blocks the calling thread.
pub struct GpuRenderer {
	device: wgpu::Device,
	queue: wgpu::Queue,
	info: wgpu::AdapterInfo,
	pipeline: wgpu::RenderPipeline,
	blend_pipeline: wgpu::RenderPipeline,
	layout: wgpu::BindGroupLayout,
}

impl GpuRenderer {
	/// Opens a device on the adapter asked for.
	pub fn new(kind: Adapter) -> Result<Self, GpuError> {
		let instance = wgpu::Instance::new(wgpu::InstanceDescriptor::new_without_display_handle());
		let adapter = block_on(instance.request_adapter(&wgpu::RequestAdapterOptions {
			power_preference: wgpu::PowerPreference::HighPerformance,
			force_fallback_adapter: kind == Adapter::Software,
			compatible_surface: None,
			apply_limit_buckets: false,
		}))
		.map_err(|_| GpuError::NoAdapter)?;
		let info = adapter.get_info();
		let (device, queue) = block_on(adapter.request_device(&wgpu::DeviceDescriptor {
			label: Some("vukem-renderer-gpu"),
			..Default::default()
		}))
		.map_err(device_err)?;

		let shader = device.create_shader_module(wgpu::ShaderModuleDescriptor {
			label: Some("layer"),
			source: wgpu::ShaderSource::Wgsl(include_str!("shader.wgsl").into()),
		});
		let uniform = |binding| wgpu::BindGroupLayoutEntry {
			binding,
			visibility: wgpu::ShaderStages::VERTEX_FRAGMENT,
			ty: wgpu::BindingType::Buffer {
				ty: wgpu::BufferBindingType::Uniform,
				has_dynamic_offset: false,
				min_binding_size: None,
			},
			count: None,
		};
		let texture = |binding| wgpu::BindGroupLayoutEntry {
			binding,
			visibility: wgpu::ShaderStages::FRAGMENT,
			ty: wgpu::BindingType::Texture {
				sample_type: wgpu::TextureSampleType::Float { filterable: false },
				view_dimension: wgpu::TextureViewDimension::D2,
				multisampled: false,
			},
			count: None,
		};
		let layout = device.create_bind_group_layout(&wgpu::BindGroupLayoutDescriptor {
			label: Some("layer"),
			entries: &[uniform(0), uniform(1), texture(2), texture(3), uniform(4)],
		});
		let pipeline_layout = device.create_pipeline_layout(&wgpu::PipelineLayoutDescriptor {
			label: Some("layer"),
			bind_group_layouts: &[Some(&layout)],
			immediate_size: 0,
		});
		// The pipeline of a layer drawn by the fixed blend state, and the one of a layer with a blend
		// mode, whose fragment shader mixes with the backdrop and so replaces the pixel.
		let make = |entry: &'static str, blend: Option<wgpu::BlendState>| {
			device.create_render_pipeline(&wgpu::RenderPipelineDescriptor {
				label: Some(entry),
				layout: Some(&pipeline_layout),
				vertex: wgpu::VertexState {
					module: &shader,
					entry_point: Some("vs_main"),
					compilation_options: Default::default(),
					buffers: &[Some(wgpu::VertexBufferLayout {
						array_stride: VERTEX_BYTES,
						step_mode: wgpu::VertexStepMode::Vertex,
						attributes: &wgpu::vertex_attr_array![0 => Float32x2, 1 => Float32x2],
					})],
				},
				fragment: Some(wgpu::FragmentState {
					module: &shader,
					entry_point: Some(entry),
					compilation_options: Default::default(),
					targets: &[Some(wgpu::ColorTargetState {
						format: FORMAT,
						blend,
						write_mask: wgpu::ColorWrites::ALL,
					})],
				}),
				primitive: wgpu::PrimitiveState::default(),
				depth_stencil: None,
				multisample: wgpu::MultisampleState::default(),
				multiview_mask: None,
				cache: None,
			})
		};
		let pipeline = make(
			"fs_main",
			Some(wgpu::BlendState::PREMULTIPLIED_ALPHA_BLENDING),
		);
		let blend_pipeline = make("fs_blend", None);

		Ok(Self {
			device,
			queue,
			info,
			pipeline,
			blend_pipeline,
			layout,
		})
	}

	/// The adapter in use: vendor, name, device type and backend, to record next to a result.
	pub fn adapter(&self) -> &wgpu::AdapterInfo {
		&self.info
	}

	/// Draws the render input with the stored textures and bitmaps of `host` into `out`, which must
	/// be exactly `width x height x 4` bytes; only the rows of the band of the input are written,
	/// as on the CPU path.
	pub fn render(&self, host: &Renderer, input: &[u8], out: &mut [u8]) -> Result<(), GpuError> {
		let r = decode(input)?;
		let layers = host.layers(&r)?;
		let mut target = Target {
			pix: out,
			width: r.output.width,
			height: r.output.height,
			rows: r.output.rows.clone(),
		};
		self.compose(
			&mut target,
			frame_fill(&r.background, r.transparent),
			&layers,
			r.output.scale,
		)
	}

	/// The GPU counterpart of `vukem_renderer::compose::compose`: fills the rows of `dst` with the
	/// background and draws the layers over them, scaling positions and sizes by `out_scale`.
	pub fn compose(
		&self,
		dst: &mut Target<'_>,
		bg: [u8; 4],
		layers: &[Layer<'_>],
		out_scale: f64,
	) -> Result<(), GpuError> {
		let (width, height) = (dst.width, dst.height);
		if dst.pix.len() != width * height * 4 {
			return Err(
				Error::new(Reason::InvalidArgument, "out_len is not the frame size").into(),
			);
		}
		let rows = dst.rows.start.min(height)..dst.rows.end.min(height);
		if rows.is_empty() {
			return Ok(());
		}
		let out = &mut *dst.pix;

		let prepared: Vec<Prepared<'_>> = layers
			.iter()
			.filter_map(|l| prepare_layer(l, out_scale))
			.collect();
		let limit = self.device.limits().max_texture_dimension_2d as usize;
		if prepared
			.iter()
			.any(|p| p.image.w > limit || p.image.h > limit)
		{
			return Err(Error::new(Reason::LimitExceeded, "render_texture_side").into());
		}

		let frame = self.device.create_buffer_init(&BufferInitDescriptor {
			label: Some("frame"),
			contents: &f32_bytes(&[width as f32, height as f32, 0.0, 0.0]),
			usage: wgpu::BufferUsages::UNIFORM,
		});
		let frame_texture = |label, usage| {
			self.device.create_texture(&wgpu::TextureDescriptor {
				label: Some(label),
				size: extent(width, height),
				mip_level_count: 1,
				sample_count: 1,
				dimension: wgpu::TextureDimension::D2,
				format: FORMAT,
				usage,
				view_formats: &[],
			})
		};
		let target = frame_texture(
			"frame",
			wgpu::TextureUsages::RENDER_ATTACHMENT | wgpu::TextureUsages::COPY_SRC,
		);
		// The frame as it was before a blend layer: the shader reads the backdrop here and writes
		// the mixed pixel to the target, which a texture cannot be at the same time.
		let backdrop = frame_texture(
			"backdrop",
			wgpu::TextureUsages::TEXTURE_BINDING | wgpu::TextureUsages::COPY_DST,
		);
		let view = target.create_view(&wgpu::TextureViewDescriptor::default());
		let backdrop_view = backdrop.create_view(&wgpu::TextureViewDescriptor::default());
		let drawn: Vec<(wgpu::BindGroup, wgpu::Buffer)> = prepared
			.iter()
			.map(|p| self.layer(&frame, &backdrop_view, p))
			.collect();
		let pitch = (width * 4).div_ceil(ROW_ALIGN) * ROW_ALIGN;
		let readback = self.device.create_buffer(&wgpu::BufferDescriptor {
			label: Some("readback"),
			size: (pitch * height) as u64,
			usage: wgpu::BufferUsages::COPY_DST | wgpu::BufferUsages::MAP_READ,
			mapped_at_creation: false,
		});

		let mut encoder = self
			.device
			.create_command_encoder(&wgpu::CommandEncoderDescriptor {
				label: Some("frame"),
			});
		// One pass clears the frame, then one pass per layer loads it: a blend layer needs the frame
		// copied to the backdrop first, and a copy cannot happen inside a pass.
		let pass = |encoder: &mut wgpu::CommandEncoder, load: wgpu::LoadOp<wgpu::Color>| {
			encoder
				.begin_render_pass(&wgpu::RenderPassDescriptor {
					label: Some("frame"),
					color_attachments: &[Some(wgpu::RenderPassColorAttachment {
						view: &view,
						depth_slice: None,
						resolve_target: None,
						ops: wgpu::Operations {
							load,
							store: wgpu::StoreOp::Store,
						},
					})],
					depth_stencil_attachment: None,
					timestamp_writes: None,
					occlusion_query_set: None,
					multiview_mask: None,
				})
				.forget_lifetime()
		};
		drop(pass(
			&mut encoder,
			wgpu::LoadOp::Clear(wgpu::Color {
				r: f64::from(bg[0]) / 255.0,
				g: f64::from(bg[1]) / 255.0,
				b: f64::from(bg[2]) / 255.0,
				a: f64::from(bg[3]) / 255.0,
			}),
		));
		for ((group, vertices), p) in drawn.iter().zip(&prepared) {
			let mixes = p.blend != BlendMode::Normal;
			if mixes {
				encoder.copy_texture_to_texture(
					target.as_image_copy(),
					backdrop.as_image_copy(),
					extent(width, height),
				);
			}
			let mut pass = pass(&mut encoder, wgpu::LoadOp::Load);
			pass.set_pipeline(if mixes {
				&self.blend_pipeline
			} else {
				&self.pipeline
			});
			pass.set_scissor_rect(
				0,
				rows.start as u32,
				width as u32,
				(rows.end - rows.start) as u32,
			);
			pass.set_bind_group(0, group, &[]);
			pass.set_vertex_buffer(0, vertices.slice(..));
			pass.draw(0..6, 0..1);
		}
		encoder.copy_texture_to_buffer(
			target.as_image_copy(),
			wgpu::TexelCopyBufferInfo {
				buffer: &readback,
				layout: wgpu::TexelCopyBufferLayout {
					offset: 0,
					bytes_per_row: Some(pitch as u32),
					rows_per_image: Some(height as u32),
				},
			},
			extent(width, height),
		);
		self.queue.submit([encoder.finish()]);

		let (tx, rx) = std::sync::mpsc::channel();
		readback
			.slice(..)
			.map_async(wgpu::MapMode::Read, move |res| {
				let _ = tx.send(res);
			});
		self.device
			.poll(wgpu::PollType::wait_indefinitely())
			.map_err(device_err)?;
		rx.recv().map_err(device_err)?.map_err(device_err)?;
		{
			let data = readback.slice(..).get_mapped_range().map_err(device_err)?;
			for y in rows {
				out[y * width * 4..(y + 1) * width * 4]
					.copy_from_slice(&data[y * pitch..y * pitch + width * 4]);
			}
		}
		readback.unmap();
		Ok(())
	}

	/// The bind group and the vertex buffer of one prepared layer.
	fn layer(
		&self,
		frame: &wgpu::Buffer,
		backdrop: &wgpu::TextureView,
		p: &Prepared<'_>,
	) -> (wgpu::BindGroup, wgpu::Buffer) {
		let img = &p.image;
		let texture = self.device.create_texture_with_data(
			&self.queue,
			&wgpu::TextureDescriptor {
				label: Some("layer"),
				size: extent(img.w, img.h),
				mip_level_count: 1,
				sample_count: 1,
				dimension: wgpu::TextureDimension::D2,
				format: FORMAT,
				usage: wgpu::TextureUsages::TEXTURE_BINDING,
				view_formats: &[],
			},
			TextureDataOrder::LayerMajor,
			&img.pix[..img.w * img.h * 4],
		);
		let copy = copy_origin(img.w, img.h, &p.placement).is_some();
		let mask = self.device.create_buffer_init(&BufferInitDescriptor {
			label: Some("mask"),
			contents: &mask_bytes(p.mask.as_ref().map(|m| m.geometry(copy)).as_ref()),
			usage: wgpu::BufferUsages::UNIFORM,
		});
		let params = self.device.create_buffer_init(&BufferInitDescriptor {
			label: Some("layer"),
			contents: &layer_bytes(img.w, img.h, p.op, p.blend),
			usage: wgpu::BufferUsages::UNIFORM,
		});
		let group = self.device.create_bind_group(&wgpu::BindGroupDescriptor {
			label: Some("layer"),
			layout: &self.layout,
			entries: &[
				wgpu::BindGroupEntry {
					binding: 0,
					resource: frame.as_entire_binding(),
				},
				wgpu::BindGroupEntry {
					binding: 1,
					resource: params.as_entire_binding(),
				},
				wgpu::BindGroupEntry {
					binding: 2,
					resource: wgpu::BindingResource::TextureView(
						&texture.create_view(&wgpu::TextureViewDescriptor::default()),
					),
				},
				wgpu::BindGroupEntry {
					binding: 3,
					resource: wgpu::BindingResource::TextureView(backdrop),
				},
				wgpu::BindGroupEntry {
					binding: 4,
					resource: mask.as_entire_binding(),
				},
			],
		});

		let q = quad_for(img.w, img.h, &p.placement);
		let mut v = Vec::with_capacity(6 * 4);
		for i in [0, 1, 2, 0, 2, 3] {
			v.extend_from_slice(&q.pos[i]);
			v.extend_from_slice(&q.uv[i]);
		}
		let vertices = self.device.create_buffer_init(&BufferInitDescriptor {
			label: Some("quad"),
			contents: &f32_bytes(&v),
			usage: wgpu::BufferUsages::VERTEX,
		});
		(group, vertices)
	}
}

fn extent(w: usize, h: usize) -> wgpu::Extent3d {
	wgpu::Extent3d {
		width: w as u32,
		height: h as u32,
		depth_or_array_layers: 1,
	}
}

/// The uniform of a layer: the texture size, the opacity and the blend mode of the shader.
fn layer_bytes(w: usize, h: usize, op: u32, blend: BlendMode) -> Vec<u8> {
	let mode: u32 = match blend {
		BlendMode::Normal => 0,
		BlendMode::Multiply => 1,
		BlendMode::Screen => 2,
		BlendMode::Overlay => 3,
		BlendMode::Add => 4,
	};
	let mut v = f32_bytes(&[w as f32, h as f32, op as f32 / 255.0]);
	v.extend_from_slice(&mode.to_le_bytes());
	v
}

/// The uniform of a layer's mask: four `vec4<f32>`, all zero when the layer has none. The first
/// holds the map of `x` and the enabled flag (1 a shape, 2 a wipe only), the second the map of `y`
/// and the direction of a wipe (1 left, 2 right, 3 up, 4 down, 0 none), the third the corner and
/// half-extents of the shape and the fourth the feather, the shape, the invert flag and the edge
/// of the wipe in pixels.
fn mask_bytes(m: Option<&MaskGeometry>) -> Vec<u8> {
	let Some(m) = m else {
		return f32_bytes(&[0.0; 16]);
	};
	let flag = |b: bool| if b { 1.0 } else { 0.0 };
	let (dir, edge) = match m.wipe {
		None => (0.0, 0.0),
		Some((Direction::Left, e)) => (1.0, e as f32),
		Some((Direction::Right, e)) => (2.0, e as f32),
		Some((Direction::Up, e)) => (3.0, e as f32),
		Some((Direction::Down, e)) => (4.0, e as f32),
	};
	f32_bytes(&[
		m.map[0] as f32,
		m.map[1] as f32,
		m.map[2] as f32,
		if m.shapeless { 2.0 } else { 1.0 },
		m.map[3] as f32,
		m.map[4] as f32,
		m.map[5] as f32,
		dir,
		m.left as f32,
		m.top as f32,
		m.half_w as f32,
		m.half_h as f32,
		m.feather as f32,
		flag(m.ellipse),
		flag(m.invert),
		edge,
	])
}

fn f32_bytes(v: &[f32]) -> Vec<u8> {
	v.iter().flat_map(|x| x.to_le_bytes()).collect()
}
