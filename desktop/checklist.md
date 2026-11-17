# Desktop release checklist

The by-hand check of the packaged desktop application (RF-213, ADR-0023 item 6,
P3-DSK-004). A person runs it on Windows 11, macOS and Linux, from the
installers attached to a release, never from a local build. NFR-040 (download
to the first exported video in under ten minutes) is timed on the way.

## How to run it

1. Use a clean machine or user account: no earlier Vukem Video install, no
   `VUKEM_VIDEO_HOME`, no workspace folder.
2. Take two builds from the release page: the one under test (`N`) and an older
   one (`N-1`) for step 8. Verify `checksums.txt` with cosign (SECURITY.md) and
   compare the SHA-256 of each file before opening it.
3. Keep a stopwatch from step 1 to the end of step 5 (NFR-040).
4. Copy the table at the end into `desktop/results/<os>/<version>.md`, one
   result per step: `pass`, `fail` with what happened, or `n/a` with the reason.
   Attach a screenshot for steps 3, 5 and 8. A failed step is a bug: write it
   in the feedback file, do not fix it during the run.

Test file for steps 4 and 5: any local video of 5 to 30 seconds with audio, for
example one made with `ffmpeg -f lavfi -i testsrc=duration=10:size=1280x720:rate=30
-f lavfi -i sine=duration=10 -c:v libx264 -c:a aac test.mp4`.

## Steps

| # | Step | Do | Expected |
| --- | --- | --- | --- |
| 1 | Install | Windows: run `vukem-video-X-Y-Z.exe` (SmartScreen: More info, then Run anyway). macOS: open the `.pkg` (Gatekeeper: control-click, Open). Linux: `apt install ./vukem-video_X.Y.Z_linux_<arch>.deb`, or run `vukem-video-X-Y-Z.AppImage`. | Installs without an administrator prompt on Windows (per user); the application shows in the Start menu, `/Applications` or the desktop menu. |
| 2 | First run | Start the application from its shortcut or menu entry. | A window opens on the project list within 30 s; no console window; no browser tab. The workspace folder exists (File, Open workspace folder opens it). The menu bar shows File, View and Help. |
| 3 | Language and version | Help, About. | The dialog says "Vukem Video" and the version under test. Menu texts follow the system language (English or pt-BR). |
| 4 | Import by dialog | Create a project. In the editor press "Choose files" and pick the test file in the native file dialog. | The system dialog opens (not a browser dialog), filtered to media files. The asset reaches "Ready" and appears in the library with a thumbnail. Dropping a file on the window also imports it. |
| 5 | Edit and export | Drag the asset to the timeline, trim it, add a text, press Export with the MP4 default and wait. | The preview plays. The export finishes with a file in the exports folder of the workspace that plays in the system player with the picture and the sound. The stopwatch of NFR-040 stops here; it reads under 10 minutes including the install. |
| 6 | Quit | File, Quit (or close the window). Check the process list. | The window closes; no `vukem-video` server process is left (`tasklist`, Activity Monitor, `pgrep`). Killing the shell from the task manager also stops the server. |
| 7 | Reopen | Start the application again. | The project list shows the project from step 5 with its edits; the media and the export are still there; undo history is intact. |
| 8 | Update to a newer build | Install `N-1` over nothing (uninstall `N` first, keep the workspace), open it, then Help, Check for updates. | With a published feed newer than `N-1`: a dialog offers "Install and restart"; accepting downloads, verifies and installs, and the application restarts as `N`. About shows `N`. If the feed is not configured (a pre-release or a local build) the dialog says so: record it as `n/a`. "Not now" leaves the application running and the version unchanged. |
| 9 | Reopen after the update | Start the updated application. | The workspace, the project and the export of step 5 are intact. "Check for updates at start-up" keeps the choice made before the update. |
| 10 | Uninstall | Windows: Settings, Apps. macOS: `sudo /usr/local/share/vukem-video/uninstall.sh`. Linux: `apt remove vukem-video`. | The program and its shortcuts go; the workspace (`%USERPROFILE%\.vukem-video` or `~/.vukem-video`) and its projects stay. |

## Results table

```text
Machine: <os and version>      Webview: <WebView2 / WKWebView / WebKitGTK version>
Build under test: <version>    Older build: <version>
Date: <yyyy-mm-dd>             Run by: <name>
NFR-040 (steps 1 to 5): <mm:ss>

| # | Result | Notes or screenshot |
| --- | --- | --- |
| 1 | | |
| 2 | | |
| 3 | | |
| 4 | | |
| 5 | | |
| 6 | | |
| 7 | | |
| 8 | | |
| 9 | | |
| 10 | | |
```

A release is not cut for the desktop until every step passes on the three
systems, or the author accepts the listed exceptions in the release notes.
