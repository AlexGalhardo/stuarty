# Third-party licences

The licence texts of everything Stuarty ships, runs or copies. [`THIRD_PARTY_NOTICES.md`](../THIRD_PARTY_NOTICES.md) lists every component and links its text here.

| Folder | Content | Kept by |
| --- | --- | --- |
| `go-modules/<module>@<version>/` | licence and NOTICE files of the Go modules the binary links | `task notices` (do not edit) |
| `npm/<package>@<version>/` | the same for the production closure of `web/package.json` | `task notices` (do not edit) |
| `crates/<crate>@<version>/` | the same for the Rust crates of the WebAssembly renderer and of `vukem-render` | `task notices` (do not edit) |
| `crate-texts/<crate>/` | the licence text of a Rust crate whose package ships none, copied from its repository | by hand, read by `task notices` |
| `fonts/` | the SIL OFL text of each bundled font | by hand, listed in `.config/third-party.json` |
| `programs/` | FFmpeg (LGPL-3.0), OpenH264, whisper.cpp and the Whisper models | by hand, listed in `.config/third-party.json` |
| `vendored/` | licences of source copied or adapted into this repository (the `golang.org/x/image/vector` rasterizer, ported to `renderer/vukem-renderer-text/src/scan.rs`; the text adapted into the built-in skills of `web/resources/skills/`, with `NOTICE-skills.md`) | by hand, listed in `.config/third-party.json` |

To change the generated folders, change a dependency or the inventory and run `task notices`; see [CONTRIBUTING](../.github/CONTRIBUTING.md).
