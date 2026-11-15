# Feature detection page

The page of SDD-012 section 21.2 and ADR-0023. It runs 18 browser tests in
the browser or webview under test and reports a verdict. The desktop shell
(BETA) is chosen with its results; nothing in the product imports this folder.

## Run

```text
go run ./desktop/detect
```

It serves `http://127.0.0.1:7979/`. Open that address in the browser or webview
under test. The page runs every test on load and on **Run again**.

Two tests need the person: the audio graph reports `running` only after a
click (use **Run again**), and the pointer test asks you to drag the box. Run
the tests once, drag the box, then click **Run again**.

**Send results** posts the JSON to the server, which writes
`desktop/results/<platform>-<webview version>-<date>.json`. **Download
results** saves the same JSON with a `data:` link.

Flags: `-addr` (default `127.0.0.1:7979`, keep it on loopback) and `-results`
(default `desktop/results`).

## What the server does

- serves `index.html`, `detect.js`, `detect.css` and `fixtures/` (embedded), with
  Range support for the fixture;
- sends the default Content-Security-Policy of the product without a nonce:
  `default-src 'self'`, so the page has no inline script, no inline style, no
  `eval`, no external request and no `blob:` URL;
- `/events` sends events numbered 1 to 100, 10 ms apart. A connection with no
  `Last-Event-ID` is cut after 40 events, so the browser reconnects with
  `Last-Event-ID: 40` and the test checks it;
- `POST /results` stores the posted JSON.

## The fixture

`fixtures/proxy.mp4` is 4 s, 540 by 960, 30 fps, H.264 baseline, AAC, with a
keyframe every 15 frames. It was made with the LGPL FFmpeg build of the project
tests (`libopenh264` is its H.264 encoder):

```text
ffmpeg -f lavfi -i "gradients=size=540x960:rate=30:duration=4:seed=3" \
  -f lavfi -i "sine=frequency=440:duration=4:sample_rate=48000" \
  -c:v libopenh264 -profile:v constrained_baseline -pix_fmt yuv420p -g 15 \
  -c:a aac -b:a 64k -shortest -movflags +faststart desktop/detect/fixtures/proxy.mp4
```

`ffprobe` reports `h264` profile `Constrained Baseline` (what an `x264`
`baseline` file reports too), 540x960, 30/1, `aac` LC at 48 kHz, and keyframes
at packets 0, 15, 30, 45, 60, 75, 90 and 105.

## The tests

`es2022`, `intl`, `tab-relay` (reported only), `css-oklch`, `fetch-range`,
`video-playback`, `video-seek`, `video-rate`, `webgl2`, `image-bitmap`,
`eventsource`, `pointer`, `audio-graph`, `resize-observer`, `crypto-uuid`,
`storage` (reported only), `fast-seek` (reported only) and `csp`. The verdict is
`pass` when every required test passes. WKWebView on macOS and WebKitGTK 6.0 on
Linux must give `pass` for BETA; WebView2 is Chromium and is run for the
record.

The browser check of the page is `web/e2e/specs/desktop-detect.spec.ts`
(`bun run e2e`, project `desktop-detect`); the server is tested by
`go test ./desktop/detect/...`.
