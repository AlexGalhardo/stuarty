# Screen recording: research

Date: 2026-11-23. Scope: competitors, the feasibility of capture in Stuarty, and local AI for editing.

Labels: **verified** means read in the cited source. **[to validate]** means it could not be confirmed. Inference is marked as such. Claims about a competitor come from its own site unless another source is cited.

## 1. Conclusions

1. No tool found combines local-first recording on several systems with an interface AI agents can operate under human approval (inference).
2. Capture is feasible through one native helper per system, supervised by the server the way FFmpeg is. Web recording is feasible over HTTPS or localhost, without global cursor and click data.
3. The largest technical risk is the macOS permission for a helper in an unsigned app. The second is global cursor and click events on Wayland, which have no generic solution.
4. Local AI is feasible with no Python in the product: whisper.cpp, which already ships, and one ONNX Runtime helper.
5. Licences exclude WhisperX as a shipped dependency in practice, Depth-Anything-V2 Base, Large and Giant, RobustVideoMatting, and the main Cap code.

## 2. Competitors

### RecMove

Source: <https://recmove.com.br/> and <https://recmove.com.br/en>. No independent review, changelog or demo was found, so every row is a claim of the site.

| Area | Claim |
| --- | --- |
| Recording | Screen and camera, automatic zoom from clicks, smoothed cursor, teleprompter |
| AI editing | Silence removal, grouped filler words and repetitions, editing by text, vertical cuts, titles and chapters through Apple Intelligence |
| Captions | More than 30 styles, word by word |
| Finishing | Colour wheels, curves, LUT, background removal without chroma key, "studio voice", private data blur |
| Agent | An AI agent can edit the video; no public API is cited |
| Platform | Apple silicon Mac (M1 or later); caption and text AI need macOS 26; Windows announced |
| Price | Free beta; free and Pro plans announced for 1.0 |
| Privacy | Runs locally, with no required account or cloud |

### Others

| Tool | Platforms | Licence | Strength | Reported problem | Source |
| --- | --- | --- | --- | --- | --- |
| Screen Studio | macOS | Proprietary [to validate] | Automatic zoom and cursor | No Windows; move to subscription (secondary source) | <https://screen.studio/pricing>, <https://trainn.co/blog/screen-studio-reviews/> |
| Loom | Mac, Windows, iOS, Android | Proprietary | Instant sharing | 5 minute limit on the free plan; price rises (secondary source) | <https://www.loom.com/pricing>, <https://trainn.co/blog/loom-reviews/> |
| OBS Studio | Windows, macOS, Linux | GPLv2 [to validate] | Free scene capture and streaming | Not researched | <https://phoronix.com/news/OBS-Studio-33.0> |
| Cap | macOS, Windows, Linux | AGPLv3, with `cap-camera*` and `scap-*` under MIT | Open source, transcription, self-hosting | Open issues on private video exposure and keyboard capture | <https://github.com/CapSoftware/Cap>, <https://github.com/CapSoftware/Cap/issues> |
| Kap | macOS | MIT | Light and open | Low recent activity [to validate] | <https://github.com/wulkano/Kap> |
| Descript | Desktop and web [to validate] | Proprietary | Editing by transcript | Slow on long projects, bugs | <https://producthunt.com/products/descript/reviews> |
| Tella | Web and desktop [to validate] | Proprietary | Automatic editing | Aggressive cuts, bugs that force a retake | <https://trainn.co/blog/tella-pricing/> |
| Recordly | macOS, Windows | [to validate] | Smooth cursor, motion blur | Not researched | <https://github.com/webadderall/Recordly> |
| Screenize | macOS | Apache-2.0 | Automatic zoom, timeline | Not researched | <https://github.com/syi0808/screenize> |

## 3. Capture feasibility

### Web

| Topic | Finding | Source |
| --- | --- | --- |
| Secure context | `getDisplayMedia` needs HTTPS and a user gesture; a VPS needs TLS | <https://developer.mozilla.org/en-US/docs/Web/API/MediaDevices/getDisplayMedia> |
| Support | Chrome 72, Edge 79, Firefox 33, Safari 13; not on Chrome for Android | <https://caniuse.com/mdn-api_mediadevices_getdisplaymedia> |
| System audio | `systemAudio: "include"` does not guarantee audio; support varies by system | MDN above |
| Long recordings | `MediaRecorder.start(timeslice)` produces chunks; persisting them is required [to validate] | <https://developer.mozilla.org/en-US/docs/Web/API/MediaRecorder> |
| Cursor constraint | [to validate] | The MDN page returned no content |

### Desktop

| System | Video | Audio | Cursor and clicks | Main limit |
| --- | --- | --- | --- | --- |
| Windows | Windows Graphics Capture from 1803, cursor control from 2004; DXGI as an alternative | WASAPI loopback in shared mode | Raw input preferred over `WH_MOUSE_LL`, which is removed silently on timeout | Yellow border needs consent to remove; protected content; antivirus with an unsigned helper [to validate] |
| macOS | ScreenCaptureKit | System audio; microphone in the stream from macOS 15 | Input Monitoring or Accessibility [to validate] | Permission attribution, ad hoc signatures losing permission after updates, helpers with no bundle not listed |
| Linux | `org.freedesktop.portal.ScreenCast` and PipeWire; `x11grab` on X11 | PipeWire [to validate] | Cursor metadata mode; clicks only through the InputCapture portal where the compositor supports it | No generic global click capture on Wayland |

Sources: <https://learn.microsoft.com/en-us/uwp/api/windows.graphics.capture.graphicscapturesession>, <https://learn.microsoft.com/en-us/windows/win32/direct3ddxgi/desktop-dup-api>, <https://learn.microsoft.com/en-us/windows/win32/coreaudio/loopback-recording>, <https://learn.microsoft.com/en-us/windows/win32/winmsg/lowlevelmouseproc>, <https://developer.apple.com/videos/play/wwdc2024/10088/>, <https://developer.apple.com/forums/thread/805245>, <https://developer.apple.com/forums/thread/795739>, <https://developer.apple.com/forums/thread/807323>, <https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.ScreenCast.html>, <https://flatpak.github.io/xdg-desktop-portal/docs/doc-org.freedesktop.portal.InputCapture.html>.

WebView capture is not validated inside the Wails shell. WebView2 exposes `ScreenCaptureStarting` (<https://learn.microsoft.com/en-us/dotnet/api/microsoft.web.webview2.core.corewebview2screencapturestartingeventargs>). WKWebView has a reported picker bug (<https://bugs.webkit.org/show_bug.cgi?id=271688>). This is a further reason to capture natively on the desktop.

### Encoding

- An LGPL FFmpeg build excludes `--enable-gpl`, `--enable-nonfree` and libx264 (<https://ffmpeg.org/legal.html>). Hardware encoders available in the pinned build are [to validate].
- A fragmented MP4 stays decodable after an interruption (<https://ffmpeg.org/ffmpeg-formats.html>), which suits crash-safe recording.
- Windows capture timestamps use QPC, a common clock for video and audio (Windows Graphics Capture documentation above).
- Proposed: one file per source with a shared clock, assembled on the timeline.

### Reusable libraries

| Library | Licence | Use | Source |
| --- | --- | --- | --- |
| `windows-capture` | MIT | Windows Graphics Capture | <https://github.com/NiiightmareXD/windows-capture> |
| `scap` | MIT | ScreenCaptureKit, WGC, PipeWire | <https://github.com/CapSoftware/scap> |
| `wasapi-rs` | MIT | WASAPI loopback | <https://github.com/HEnquist/wasapi-rs> |
| `cpal` | Apache-2.0 | Audio devices | <https://github.com/RustAudio/cpal> |
| Cap main code | AGPLv3 | **Unusable** | <https://github.com/CapSoftware/Cap> |

## 4. Local AI

| Tool | Code | Weights | Usable | Runtime | Proposed role |
| --- | --- | --- | --- | --- | --- |
| [whisper.cpp](https://github.com/ggml-org/whisper.cpp) | MIT | MIT | Yes, already used | Separate process | Required |
| [WhisperX](https://github.com/m-bain/whisperx) | BSD-2-Clause | Per-language aligners; gated pyannote for diarization | Conditional | Python and PyTorch | Not recommended (ADR-0021 rejected it) |
| [YuNet](https://github.com/opencv/opencv_zoo/tree/main/models/face_detection_yunet) | MIT | MIT per the README; training data origin [to validate] | Yes | ONNX | Required for vertical reframing |
| [BEN2 base](https://github.com/PramaLLC/BEN2) | MIT | "Open source", no separate weights licence text [to validate] | Conditional | ONNX | Optional, after confirming the weights licence |
| [MediaPipe Face Landmarker](https://developers.google.com/edge/mediapipe/solutions/vision/face_landmarker) | Apache-2.0 | Apache-2.0 | Yes; desktop runtime [to validate] | Tasks API | Optional, only if YuNet tracking is not enough |
| [Depth-Anything-V2 Small](https://github.com/DepthAnything/Depth-Anything-V2) | Apache-2.0 | Apache-2.0 | Yes | Community ONNX port | Optional, for parallax and depth blur |
| Depth-Anything-V2 Base, Large, Giant | Apache-2.0 | CC-BY-NC-4.0 | **No** | PyTorch | Excluded |
| [ONNX Runtime](https://github.com/microsoft/onnxruntime) | MIT | n/a | Yes | CPU, DirectML, CoreML, CUDA | Required, one helper |
| [DeepFilterNet](https://github.com/Rikorose/DeepFilterNet) | MIT or Apache-2.0 | Same | Yes | Rust | Optional voice denoise |
| [RobustVideoMatting](https://github.com/PeterL1n/RobustVideoMatting) | GPL-3.0 | Undeclared | **No** | PyTorch | Excluded |

CPU speed and quality of these models on the target machines were not measured. They need a spike before a model is committed to a feature.

## 5. Spikes on real hardware

1. Windows 10 and 11: WGC, loopback, multi-monitor, HDR, protected content, antivirus with the unsigned helper.
2. macOS 13 to 26: helper signed, ad hoc and notarized; repeated permission prompts.
3. Linux GNOME and KDE on Wayland, and X11: restore token, cursor metadata.
4. `getDisplayMedia` inside the three WebViews of the shell.
5. CPU time of YuNet, BEN2 base and Depth-Anything-V2 Small on an ordinary PC.

## 6. External motion engines

Author decision (2026-11-23): only HyperFrames has official support as an external motion engine.

| Engine | Licence | Position | Source |
| --- | --- | --- | --- |
| HyperFrames | Apache-2.0; 0.x, npm `hyperframes` 0.9.86 at research time | Supported through the flow of epic 33. The product never installs, bundles or runs it (ADR-0042). Node, Chrome and an LGPL FFmpeg are installed by the user | <https://github.com/heygen-com/hyperframes>, <https://hyperframes.heygen.com/concepts/determinism> |
| Remotion | Proprietary, with a bundled GPL FFmpeg | Out, with no skill and no bundling (ADR-0027). A clip rendered with it can still be imported like any external render | <https://www.remotion.dev/docs/license/faq>, <https://www.remotion.dev/docs/miscellaneous/ffmpeg-license> |

- The hosted HeyGen MCP connector stays out (ADR-0032).
- Whether HyperFrames accepts a custom FFmpeg path is [to validate]. Epic 73 stops if the only option is a GPL build on the project's side.
- Epic 33 recorded its real round as not verified. Epic 73 runs it.
