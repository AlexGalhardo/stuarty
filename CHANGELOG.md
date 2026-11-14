# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **timeline:** Add one invalid example per violation code and valid examples for the ALPHA features (keyframes, transitions, shapes, slots, per-word highlight)

## [0.2.0] - 2026-11-14

### Added

- **repo:** Add the Go module, the package skeleton and a vukem-video serve that boots (#28) ([2359314](https://github.com/AlexGalhardo/vukem-video/commit/2359314d922abf43978b0b0d7ef41ae96938f939))
- **core:** Add timeval with exact frame arithmetic and strict time parsing (#30) ([b9c79af](https://github.com/AlexGalhardo/vukem-video/commit/b9c79afccbb38f743e157b8ab0141f8a26d65b2e))
- **service:** Add the workspace directory, lock, runtime.json, local token and Open ([0a0b3db](https://github.com/AlexGalhardo/vukem-video/commit/0a0b3db63d13655cdd8d7e7a9871e221988f0aac))
- **timeline:** Add document types, canonical encoding, identifiers, validation and migrations ([da70c9b](https://github.com/AlexGalhardo/vukem-video/commit/da70c9b0e7e646fc8ade60d6a890b6cd7ea1b920))
- **service:** Declare the persistence interfaces ([a01c3f2](https://github.com/AlexGalhardo/vukem-video/commit/a01c3f2c42216136443d3a8457cee279c9d808e1))
- **web:** Add the editor schema migrations ([e1b94e9](https://github.com/AlexGalhardo/vukem-video/commit/e1b94e915e7fcf686730d7f9e8c11908242b386e))
- **web:** Add the store over the vukem database ([6ffd527](https://github.com/AlexGalhardo/vukem-video/commit/6ffd5272844d474ad403fb074037284b5dd6f3f2))
- **ui:** Add the projects page, dark default theme and context based translations ([30b8372](https://github.com/AlexGalhardo/vukem-video/commit/30b83722a74deb12c794c1377a6ac6aef28a835e))
- **engine:** Add the ffargs grammar, the typed builders and the argv checker ([dcd9198](https://github.com/AlexGalhardo/vukem-video/commit/dcd919856a4275eb979e6fe677f97478fd9576fc))
- **engine:** Discover FFmpeg and classify its licence ([cd78784](https://github.com/AlexGalhardo/vukem-video/commit/cd78784c6ebabf23d593b5aaf189d6f11d113d3b))
- **engine:** Add the FFmpeg runner with deadlines, stall watchdog and tree kill ([55c840a](https://github.com/AlexGalhardo/vukem-video/commit/55c840a44041be65ef7a78562771f238bb6eaa41))
- **engine:** Probe media with a signature sniff and the FF-06 checks ([b2e2c50](https://github.com/AlexGalhardo/vukem-video/commit/b2e2c50fb9382ac3630fa59dcd60601ddaabf2a9))
- **engine:** Read FFmpeg capabilities and detect the H.264 encoder by test encode ([9dae9ff](https://github.com/AlexGalhardo/vukem-video/commit/9dae9ffe704b5325577756508e33c8905d2b5ff8))
- **site:** Copy the vukem site shell and add the first docs pages ([03d1548](https://github.com/AlexGalhardo/vukem-video/commit/03d154855455a23f40d307d3709e53e635bc62e1))
- **site:** Add the landing page in English and Brazilian Portuguese ([be80c18](https://github.com/AlexGalhardo/vukem-video/commit/be80c187ba442c490ada2bdce5eabcab26b3e6d9))
- **timeline:** Add Clone, Raw and the related objects of a violation ([6c50600](https://github.com/AlexGalhardo/vukem-video/commit/6c5060032fcc5e56e22f12ce55d054117e2d55f7))
- **command:** Add the registry, Apply, errors, diff, primitives, conflicts and DescribeDiff ([4ffc1df](https://github.com/AlexGalhardo/vukem-video/commit/4ffc1df5a5c94c082b54bb33decedecc502fa1a9))
- **raster:** Bundle the six font families and add the font set ([a41c386](https://github.com/AlexGalhardo/vukem-video/commit/a41c3865c9a5d295ec687b7430c42a2f02d4b2f2))
- **raster:** Shape, lay out and draw text, captions and shapes ([c843022](https://github.com/AlexGalhardo/vukem-video/commit/c8430222acd1ef8d93385773aa41140dff580b40))
- **raster:** Add the bitmap cache and the caption benchmarks ([65af56f](https://github.com/AlexGalhardo/vukem-video/commit/65af56fef7460f9412d69e8cbe82021c72203e96))
- **engine:** Add the content-addressed store ([37a503a](https://github.com/AlexGalhardo/vukem-video/commit/37a503aa61e3a1249f98914de66c7ff010634cdb))
- **engine:** Add the ingest plan, probe summary, peaks and analysis parsers ([40352d1](https://github.com/AlexGalhardo/vukem-video/commit/40352d17f1b20d113838914862dfeaba9b5a7a22))
- **engine:** Allow the packet flags probe in the ffprobe grammar ([47e3d4c](https://github.com/AlexGalhardo/vukem-video/commit/47e3d4ce0440726dbbf62d8cdb00ea055d6eec73))
- **engine:** Export Scrub and add the BT.709 setparams builder ([1d85449](https://github.com/AlexGalhardo/vukem-video/commit/1d854495552520bea35b25eeec5774a23fbb33c4))
- **engine:** Add the ingest pipeline ([53e434e](https://github.com/AlexGalhardo/vukem-video/commit/53e434e9a4ec580145788d7b9b0befcd22047609))
- **ui:** Apply the design tokens, the focus rule and the reduced-motion rule ([0f8b95a](https://github.com/AlexGalhardo/vukem-video/commit/0f8b95a789320ff3194cdf7916896fd7a62f80d5))
- **ui:** Add the shadcn components the design system lists ([f2c5a05](https://github.com/AlexGalhardo/vukem-video/commit/f2c5a05fcf2cf0cb934d51797398b137522651ab))
- **ui:** Add the contrast check of the design tokens ([dcab797](https://github.com/AlexGalhardo/vukem-video/commit/dcab7972eadf3670279c0b35b94bfe67d6112706))
- **timeline:** Add style maps, font weights, AssetDuration and CloneItem for the commands ([981a1bf](https://github.com/AlexGalhardo/vukem-video/commit/981a1bfa39e723f88129256edf4431eafd4b3388))
- **command:** Add the shared rules of the Prototype commands ([b908db0](https://github.com/AlexGalhardo/vukem-video/commit/b908db066d02081da99e598f3d5eff707f5b5225))
- **command:** Add project.rename, settings.set and asset.add with their golden cases ([a461fee](https://github.com/AlexGalhardo/vukem-video/commit/a461fee13290cc5e1417eea24207fa93aca79397))
- **command:** Add the track commands with their golden cases ([4199039](https://github.com/AlexGalhardo/vukem-video/commit/419903920fe3cc3b20019757c7b38a5467cbb2ef))
- **command:** Add item.move, item.trim, item.split and item.delete with their golden cases ([c31fdb6](https://github.com/AlexGalhardo/vukem-video/commit/c31fdb699e9f4e942ac755b32344176a1a4f75f3))
- **command:** Add clip.add, text.add, text.set and caption.add with their golden cases ([d7da282](https://github.com/AlexGalhardo/vukem-video/commit/d7da28241415eb5d31751e875365cee7c686f88e))
- **timeline:** Generate the JSON Schema of the document from the Go types ([3cc30d7](https://github.com/AlexGalhardo/vukem-video/commit/3cc30d7a78329090eef46014cae16ddc0555b897))
- **schema:** Add contractgen and the generated contracts ([13c6da7](https://github.com/AlexGalhardo/vukem-video/commit/13c6da7ae92791dab6a48d05bd9bf1fbc7233223))
- **ui:** Generate the TypeScript contracts and add task codegen with a CI check ([dd37ce6](https://github.com/AlexGalhardo/vukem-video/commit/dd37ce66287b5711a2ee31c043699ad90899775c))
- **lint:** Add the finding, the rules of the Prototype and New ([1ceb413](https://github.com/AlexGalhardo/vukem-video/commit/1ceb413b5e9a27c0a4ab0ae256d43e087567bf3e))
- **lint:** Render the lint report as text ([6972142](https://github.com/AlexGalhardo/vukem-video/commit/69721428b9e2df20d8ca409cee6a1ba2a3251cce))
- **plan:** Add Frame, the safe area and the evaluation of an item ([0dd9c20](https://github.com/AlexGalhardo/vukem-video/commit/0dd9c20ed00770e82d8c74d963bb4e573bb6e8c1))
- **plan:** Add the audio plan ([94c2581](https://github.com/AlexGalhardo/vukem-video/commit/94c25811bc661ea5b9d63f5310364596b31872c7))
- **plan:** Add Window, the compact plan of SDD-004 section 10 ([bcec53a](https://github.com/AlexGalhardo/vukem-video/commit/bcec53a3de207887609163de778470906d26875f))
- **web:** Add vukem-video open ([310da18](https://github.com/AlexGalhardo/vukem-video/commit/310da180741da915faaaa80ccc96eabad5a9b665))
- **installers:** Add the installer icon and the tool that draws it ([ea227d8](https://github.com/AlexGalhardo/vukem-video/commit/ea227d81e759ece86d4299d202752f48b2d83375))
- **installers:** Add the Windows installer and the pinned tool fetcher ([d29b327](https://github.com/AlexGalhardo/vukem-video/commit/d29b32755a33359db2e0f7549a9b98f7f53352ec))
- **installers:** Add the Debian package, the AppImage and the macOS package ([865d651](https://github.com/AlexGalhardo/vukem-video/commit/865d651f881c1bc261431d9383c7a63b72e40613))
- **installers:** Prove the installers carry the binaries of the release ([dee867c](https://github.com/AlexGalhardo/vukem-video/commit/dee867c88296ca85c5cd6a2bf815a12f81d3b5b9))
- **timeline:** Enforce the shipped font weights of the bundled families ([caae428](https://github.com/AlexGalhardo/vukem-video/commit/caae428bb8ebfbbf8e6f2ae2aed60a91b15a3a29))
- **command:** Check the effective font weight against the shipped weights ([1235200](https://github.com/AlexGalhardo/vukem-video/commit/12352009b472d6544efadc1789225d5cea85fb3d))

### Changed

- **BREAKING:** **repo:** Replace the Studio application with the Vukem Video editor ([ac702d2](https://github.com/AlexGalhardo/vukem-video/commit/ac702d2af50d2e26f8bae4da1b45cdc63c5d98d5))

### Fixed

- **timeline:** Resolve the golangci-lint findings ([894409a](https://github.com/AlexGalhardo/vukem-video/commit/894409ae9859faa1f54e27b742f068e8c0379870))
- **service:** Justify the gosec G703 finding on the lock file open ([60fa752](https://github.com/AlexGalhardo/vukem-video/commit/60fa752f3de62da8aadba724c695db8378426bd1))
- **service:** Let the 0700 workspace test create its directory ([56f834b](https://github.com/AlexGalhardo/vukem-video/commit/56f834bbcace5773e121494f41eb9594b75b3550))
- **site:** Strip tags until none is left and compare hosts with the URL parser ([743a23b](https://github.com/AlexGalhardo/vukem-video/commit/743a23b903848e135518a8874e39a38c9f9c03df))
- **ingest:** Bound the thumbnail sheet by the plan, not by what FFmpeg wrote ([b50fd59](https://github.com/AlexGalhardo/vukem-video/commit/b50fd595f9bd2584f44be8677534fbe93b95820e))
- **raster:** Use deterministic trigonometry in the geometry ([3ad6c5f](https://github.com/AlexGalhardo/vukem-video/commit/3ad6c5f901b5d794ed9ade25c785546f8fe64fb0))
- **raster:** Fill with a portable rasterizer and round outline points ([85e6ed7](https://github.com/AlexGalhardo/vukem-video/commit/85e6ed70b6ba67e1f0dc47b8ecf52237905e6f1b))
- **release:** Run prepare on the pushed tip of the release branch ([98ef157](https://github.com/AlexGalhardo/vukem-video/commit/98ef1573e36930b058713fbd8720ad855a8330c5))
- **deps:** Update vukem to v0.44.0 ([79acc0d](https://github.com/AlexGalhardo/vukem-video/commit/79acc0dbb3b585866bea6926f3d1c8cfdaa6a4de))

### Security

- **deps:** Update golang.org/x/image to v0.38.0 ([35b6b7a](https://github.com/AlexGalhardo/vukem-video/commit/35b6b7a63cc141b8f6d69dfe2da2476e2af88b52))

## [0.1.0] - 2026-11-07

### Added

- **ui:** Add a light and dark theme toggle ([cfc4d74](https://github.com/AlexGalhardo/vukem-video/commit/cfc4d74b317e72e8460ee93a2e52c7548bb58189))
- **auth:** Allow only two Google accounts to sign in ([a4fbdff](https://github.com/AlexGalhardo/vukem-video/commit/a4fbdffbe1aded8e24716f4659f79d1bb1340c91))
- **dashboard:** Send the brief form options and latest jobs to the dashboard ([85f28f5](https://github.com/AlexGalhardo/vukem-video/commit/85f28f50dbb176fef2d2f0e6ce2a9c4613705368))
- **ui:** Add shadcn/ui with a collapsible sidebar layout and tooltip theme toggle ([4ec05da](https://github.com/AlexGalhardo/vukem-video/commit/4ec05dae0d8cbebc7f3d00cf39cbd368d5a93afe))
- **login:** Redesign the login page with a branded panel and one Google button ([2e0b008](https://github.com/AlexGalhardo/vukem-video/commit/2e0b00813d0b27ba8fc90de71d81c6dbe150327d))
- **dashboard:** Rebuild the dashboard with the brief form, recent videos, field tooltips and tips ([427f94f](https://github.com/AlexGalhardo/vukem-video/commit/427f94f27f4d8f38eb7b75b91b9dd59401d833ff))
- **i18n:** Translate the login page, sidebar and dashboard into English and Brazilian Portuguese ([fcb504b](https://github.com/AlexGalhardo/vukem-video/commit/fcb504b2094a8b144015294e9f4093322489b839))
- **ui:** Collapse the sidebar to an icon rail and remember it across reloads ([bfa6072](https://github.com/AlexGalhardo/vukem-video/commit/bfa6072714483e7d2e02450478cc48c24b63cca2))
- **dashboard:** Show example placeholders in the brief fields ([9c2ff55](https://github.com/AlexGalhardo/vukem-video/commit/9c2ff557fd9c238953cd05e440b785929fbd5176))
- **routes:** Serve the dashboard only at / and redirect /dashboard there ([89c44ed](https://github.com/AlexGalhardo/vukem-video/commit/89c44ed8f7bd0f1feca7d6b6adaf559165da9614))
- **film:** Add local dual-format launch film production ([fabdde5](https://github.com/AlexGalhardo/vukem-video/commit/fabdde53dcfa8bf8622a28f70a8e8aec3ddd2e92))
- **film:** Open on the CLI and close on the finance app ([4461c61](https://github.com/AlexGalhardo/vukem-video/commit/4461c614d2157ed461bcdda76be2e57324866d30))
- **site:** Migrate the docs site from Next.js to Astro on the vukem site shell ([16def7f](https://github.com/AlexGalhardo/vukem-studio/commit/16def7f789c268162f9cb85be89148d7c9cda2ac))
- **studio:** Add Claude and OpenAI planners with a key check, a cost cap and a real-model test ([860e8a3](https://github.com/AlexGalhardo/vukem-studio/commit/860e8a33d27c9701073e67ec2e079b9d92bb6131))
- **deploy:** Ship the skill files, a queue worker setting and a migrate pre-deploy step in the app image ([5a5821a](https://github.com/AlexGalhardo/vukem-studio/commit/5a5821aa7ed7a4c31c6b7e6f59d7051d477721cf))
- Raise the brief duration limit to 300 seconds and scale plan, token and queue limits with it ([e9e2422](https://github.com/AlexGalhardo/vukem-studio/commit/e9e2422a7d61dc3faa413cb6b10e05943b44e51e))
- **studio:** Render a brief into a checked mp4 ([64d9289](https://github.com/AlexGalhardo/vukem-studio/commit/64d92891d656731a5ab0dd852cf48021c9ff2cb6))
- Add the vukem app shell and its ci jobs ([9c02185](https://github.com/AlexGalhardo/vukem-studio/commit/9c02185ffbb1951e79d746f6ce12ab0448093816))

### Fixed

- **film:** Validate production sources and reuse art direction tokens ([0339a6d](https://github.com/AlexGalhardo/vukem-video/commit/0339a6d2190ee23882c5c2d2da76dd4eb3fe8778))
- **site:** Date doc pages by author date so rebase merges keep them valid ([ea230fd](https://github.com/AlexGalhardo/vukem-studio/commit/ea230fdafd1e50e3950ffd72f62d267db9946197))
- **app:** Remove the public posts scaffold ([9e2fda7](https://github.com/AlexGalhardo/vukem-studio/commit/9e2fda76ba5c44f6ae0ff4c211938036c4aef6af))
- **deploy:** Migrate the SQLite volume in the start command, not in pre-deploy ([e402fb5](https://github.com/AlexGalhardo/vukem-studio/commit/e402fb563056625923c378cd0fc1f75669552a73))
- **deploy:** Let each Railway service pick its Dockerfile instead of the shared railway.toml ([d276ad2](https://github.com/AlexGalhardo/vukem-studio/commit/d276ad283bc55fcc65f6eaa0150d59dcf8882ab6))
- **deploy:** Move the vukem queue tables out of the way of the app's jobs table ([658fcb5](https://github.com/AlexGalhardo/vukem-studio/commit/658fcb50dbc570bf9d8d59871bb6845cf4760519))
- **site:** Remove the legacy pt-br redirect that allowed an open redirect ([9fc34c6](https://github.com/AlexGalhardo/vukem-studio/commit/9fc34c6a1fdbe5733b804854f1aa09dfa6bc6381))
- **site:** Answer 400 on malformed paths and keep decoded paths inside dist ([33d5c16](https://github.com/AlexGalhardo/vukem-studio/commit/33d5c167703a37071ca7c35cd267c563b91e8b81))
- **studio:** Bound every encoder process with a deadline and reject caption injection in plans ([3414db3](https://github.com/AlexGalhardo/vukem-studio/commit/3414db35f29501ca8d71a6709c847bd1d58be8ef))
- **jobs:** Enforce the rolling quota, claim and requeue with atomic SQL ([d32e230](https://github.com/AlexGalhardo/vukem-studio/commit/d32e230db5869a99aa9ac9cc900e25a0a2df6c8d))
- **docs:** Use a markdown link on the railway page ([cae7a8f](https://github.com/AlexGalhardo/vukem-studio/commit/cae7a8f5f15db84ca6c01e059ac71fe2919e0f8b))

### Security

- **security:** Honor the declared WAV data size and check the decoded voice length ([c415fd2](https://github.com/AlexGalhardo/vukem-studio/commit/c415fd280bcbcd0c28379380ceecaad22ae1ab42))
- **security:** Save job columns instead of whole stale rows ([a4fa430](https://github.com/AlexGalhardo/vukem-studio/commit/a4fa430dad56f41ff7faa7d82df523d2e050b25a))
- **security:** Stop following symlinks in the root chown of the start command ([50e5e09](https://github.com/AlexGalhardo/vukem-studio/commit/50e5e09a552702895aaeeedad75b4c2819e40e0f))
- **security:** Remove password sign-in so Google is the only way in ([b2063f4](https://github.com/AlexGalhardo/vukem-studio/commit/b2063f47671ac059bc83b790a24573aaf0df77e1))
- **security:** Count retries in the rolling render quota ([1bc8114](https://github.com/AlexGalhardo/vukem-studio/commit/1bc81140afc4efcc346d68dd16bd7e91abbc15f4))
- **security:** Add a global rolling daily model budget ([7d3afc4](https://github.com/AlexGalhardo/vukem-studio/commit/7d3afc46c8da935e36ad5bcd179ea12a474d0423))

[Unreleased]: https://github.com/AlexGalhardo/vukem-video/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/AlexGalhardo/vukem-video/releases/tag/v0.1.0
