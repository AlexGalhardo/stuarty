# Epic 21 on Windows 11: the background mode, run for real (2026-11-20)

Build: `stuarty-desktop.exe` with `-H windowsgui -X main.version=0.0.0-epic21` (from `desktop/shell/app`) and
`stuarty.exe` from `./web/cmd/app`, both from the branch `epic/21-desktop-background-service`. WebView2 157.0.4341.60.

Setup, all in private folders and nothing of the machine touched:

- The workspace is a private folder. The settings of the shell live in `%APPDATA%\stuarty\background.json`, and
  `APPDATA` pointed to a private folder for the process, with `{"keep_running":true,"start_at_sign_in":false,"asked":true}`.
- The network is a fake on loopback (the `fake` connector, `STUARTY_TEST_SOCIAL_URL=http://127.0.0.1:7943`), a
  small server that logs every request. No real account, no real network.
- The workspace was prepared with the browser suite on its own port: the fake account connected, a project with a
  seeded export, one publication planned for 02:14 local time (05:14 UTC), five minutes ahead, and approved in the
  queue. Then that server stopped and the desktop shell was started on the same workspace.
- The window was closed the way a person closes it, with `Process.CloseMainWindow` (a `WM_CLOSE`), and its state
  read with `IsWindowVisible`. No click, no key, no screenshot. The start at sign-in was **not** registered, and no
  Run key, scheduled task or service was written or started on this machine.

| Step | Result |
| --- | --- |
| Start the shell; `POST /api/v1/system/scheduler {"action":"status"}` with the workspace token | `{"paused":false,"scheduled":1}` |
| The same with `"action":"pause"`, with the token alone | 403 `E_FORBIDDEN`, "only the desktop shell pauses or resumes publishing" |
| Close the window with the option on and the post scheduled | the window is hidden (`visible=False`); `stuarty-desktop.exe` and its `stuarty.exe` child keep running; `scheduled` is still 1 |
| Start a second copy with the same workspace | it exits with code 0 at once; still one `stuarty-desktop.exe` and one child (the other `stuarty.exe` processes of the machine have other parents); the first window is visible again and focused (`visible=True`) |
| Close the window again and wait for 02:14 | the fake network logged `POST /videos` at 05:14:00.020Z, once; the publication is `published`, `attempts` 1, with the URL the fake returned; `scheduled` is 0 |
| Show the window and close it with nothing scheduled | the process exits; the child is gone with it |

Not run, and why:

- **Sign out and in with start at sign-in on.** The registration is the Run key of the user; the author asked that no
  registration be made on this machine. The code path is tested on a scratch registry key (`go test ./desktop/service/`)
  and by a check that the installer script removes the same key and value; the real round trip is a manual step for a
  disposable machine (the question "Epic 21: start at sign-in" in the questions file lists it).
- **The tray icon and its menu** (Open, Pause publishing, Quit with the warning): a tray menu cannot be driven without
  screen automation, which is not used here. The decisions behind them are unit tests (`DecideClose`, `QuitPrompt`,
  `QuitWaitingMessage`), and the server side of Pause is tested on the route and on the scheduler. The icon was not looked at.
- **With the option off, closing exits**: shown here for nothing scheduled, not for the option off with a post scheduled
  (the unit test `TestDecideClose` covers that case).
- **The installer.** Inno Setup is not installed on this machine: the installer and the uninstaller were not built or run.
