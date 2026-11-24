# Face detection with YuNet through ONNX Runtime, 2026-11-24

Written by epic 70 (task 2) before the feature is promised to anyone (ADR-0050, SDD-024 section 8). This is not a `perfreport` table: it measures the speed and the detection rate of the helper on one machine, and no build fails on it.

## Setup

| Item | Value |
| --- | --- |
| Machine | windows/amd64, AMD Ryzen 7 5700X3D (8 cores, 16 threads), NVIDIA GeForce RTX 4060, driver 32.0.16.2124 |
| Load | **busy**: the processor reported 96% load during the runs, because other agents of the team were building and testing; the numbers are therefore a floor for an idle machine |
| Helper | `stuarty-aihelper` 0.0.0, `ort` 2.0.0-rc.15 in `load-dynamic` mode, release build, 274 KB |
| Runtime | ONNX Runtime 1.24.4 with the DirectML provider (`Microsoft.ML.OnnxRuntime.DirectML` 1.24.4 from nuget.org), `onnxruntime.dll` 17.3 MB; one library serves CPU and DirectML |
| Model | `face_detection_yunet_2023mar.onnx`, 232,589 bytes, fixed input 1 x 3 x 640 x 640, from opencv_zoo commit `48b10fa505aeb51fbd127fc60a5c660040e823b7` |
| Input | the decoded frame scaled to fit 640 x 640 with its aspect kept, padded at the bottom or the right, RGB24 through a pipe |
| Fixture A | `aihelper/testdata/make-face-fixture.sh`: a drawn face crossing a 1280 x 720 frame, 10 s at 10 fps, 100 frames, 257,817 bytes (the same bytes on every run). It is authored by the project, so it carries no third-party licence |
| Fixture B | a 720p, 10 s, 30 fps clip of an animated short film (no real face): 300 frames, for speed only |

## Speed

`infer_ms` is the time the helper spent on the tensors and the decoding of all frames (what a faster FFmpeg or a faster pipe cannot change); `wall` includes FFmpeg decoding and scaling, the start of the process and the creation of the session.

| Fixture | Provider | Frames | `infer_ms` | Frames per second (inference) | `wall` |
| --- | --- | --- | --- | --- | --- |
| A | CPU | 100 | 479 | 209 | 0.98 to 1.7 s (3 runs) |
| A | DirectML | 100 | 381 | 262 | 3.1 to 4.8 s (3 runs) |
| B | CPU | 300 | 2070 | 145 | 2.3 s |
| B | DirectML | 300 | 1181 | 254 | 3.2 s |

Reading:

- The CPU alone runs the model at more than 140 frames per second on a loaded machine, so the feature is usable without a graphics device: a one-hour video analysed at 5 frames per second is 18,000 frames, about two minutes of inference plus the decoding.
- DirectML is faster per frame once the session exists, and costs two to three seconds to create it. It pays back on a long video and not on a short one. The order of ADR-0050 (DirectML, then CPU) stays, and the helper reports which provider it used so the job message can say it.
- Not a STOP condition: CPU speed does not make the feature unusable.

## Detection rate

| Fixture | Provider | Frames with at least one face | Score |
| --- | --- | --- | --- |
| A | CPU | 100 of 100 | 0.85 to 0.89 in the first frames |
| A | DirectML | 100 of 100 | same boxes |
| B | CPU and DirectML | 0 of 300 | no real face in it: no false positive either |

On fixture A, in all 100 frames, the horizontal centre of the detected box stayed within 4.0 pixels (in the 640-pixel frame) of the centre the generator drew, and the vertical centre within 12.7 pixels (the box includes the hair, which sits above the drawn skin ellipse). A drawn face is not a person: the detection rate on real footage (glasses, a turned head, a hand in front of the mouth, poor light) is **not measured here**, because the epic's licensed face video was not available to the agent and no third-party media was downloaded. YuNet's own documentation gives 0.834 average precision on the easy subset of WIDER Face and says it finds faces between about 10 and 300 pixels; at a 640-pixel frame that is a face of at least 3% of the width of a 1080p source.

## What the numbers decide

- One ONNX Runtime library (the DirectML build) serves both providers: no second download.
- The model input is fixed at 640 x 640, so the host scales and pads the frame and maps the boxes back (SDD-024 section 5).
- The detection rate on real footage must be measured with a licensed face video before the feature is called finished; until then the acceptance test of the "face stays in frame" criterion runs on tracks from the drawn fixture and on golden tracks.
