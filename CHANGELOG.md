# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.4.0] - 2026-11-15

### Added

- **service:** Add the import path guard of SDD-007 section 7.1 ([f18d317](https://github.com/AlexGalhardo/vukem-video/commit/f18d317a814b550e42b245d36a023e0e95614f40))
- **service:** Report the free space of the workspace volume ([1352b3a](https://github.com/AlexGalhardo/vukem-video/commit/1352b3a9a7f9354ebffa078ffa4be1477b7f9824))
- **service:** Let SetFacts record the kind the probe gave an asset ([aa13b59](https://github.com/AlexGalhardo/vukem-video/commit/aa13b59ed3c840e3dc7f7ba090076654725f6144))
- **service:** Render the texts of media lists, imports and jobs ([0ee948a](https://github.com/AlexGalhardo/vukem-video/commit/0ee948ac4ce5ae1ae4ea4d09e936be0043dd71c5))
- **service:** List the queued jobs older than an instant ([f86e3d9](https://github.com/AlexGalhardo/vukem-video/commit/f86e3d92d145e2c5f8a63ffb0813f8065cab7cdb))
- **service:** Import media, run ingest and export jobs, serve asset files ([0b1c07f](https://github.com/AlexGalhardo/vukem-video/commit/0b1c07ff3b8e2a453db25b3cfa83d013e0816a6b))
- **web:** Connect the job machinery to the vukem queue ([41eb6c4](https://github.com/AlexGalhardo/vukem-video/commit/41eb6c41ef624ce4f8bca788c68a049bfe5e62d6))
- **service:** Reach media import, media and job reads over HTTP from the client ([ba5dcf4](https://github.com/AlexGalhardo/vukem-video/commit/ba5dcf4eb1958b9b6dcddb581826a9ceac1b45fe))
- **repo:** Add perfreport, the performance table and its regression check ([c70ddfe](https://github.com/AlexGalhardo/vukem-video/commit/c70ddfeb7162d0110fa4e6bda7ba1907337f3d02))
- **mcp:** Add the MCP server with the Prototype tools that have a Service method ([4506c72](https://github.com/AlexGalhardo/vukem-video/commit/4506c72c1c017f9025dc49f5878cd6bd1f001226))
- **cli:** Add the mcp command ([715a025](https://github.com/AlexGalhardo/vukem-video/commit/715a0256a30541cb2fee0d48e45ccabbe602d2c9))
- **command:** Add item.rename, item.set_transform, item.set_transition and item.set_audio ([392c262](https://github.com/AlexGalhardo/vukem-video/commit/392c2625ca4dad957d5965a785692fe74bb0826b))
- **command:** Add item.duplicate, item.crossfade and timeline.close_gap ([5805586](https://github.com/AlexGalhardo/vukem-video/commit/580558615d14e90f0c36a0a402e4526da74b53dd))
- **command:** Add audio.add, clip.set_framing and clip.set_color ([9d585bc](https://github.com/AlexGalhardo/vukem-video/commit/9d585bc6a2e03f05e283fb7876b6fb462a8b4152))
- **raster:** Add FontSet.Missing, shape and highlight goldens and the font licence column ([a082e42](https://github.com/AlexGalhardo/vukem-video/commit/a082e42728d2b89f0d32d5ff51e38bedcb73784d))
- **command:** Add clip.set_speed ([407d250](https://github.com/AlexGalhardo/vukem-video/commit/407d2507827f0b8a978f642fe918577b46e7060a))
- **mcp:** Add media_import, media_inspect, job_start and job_get ([50d0d7c](https://github.com/AlexGalhardo/vukem-video/commit/50d0d7cbf65a8efc17384339b5fa174dbb3b700f))
- **cli:** Add media:import, media:inspect, media:list, export, job:show and job:list ([2817a57](https://github.com/AlexGalhardo/vukem-video/commit/2817a57f8152e8aff2f66cb0ecdd87c107542000))
- **mcp:** Add the state filter to job_get when listing by project ([3e18bd0](https://github.com/AlexGalhardo/vukem-video/commit/3e18bd0f54cdfa649ae824d79a2b7e7014a1fb67))
- **ui:** Wire the editor client to the real HTTP API ([788a3df](https://github.com/AlexGalhardo/vukem-video/commit/788a3df0c3205557ba2cedda897270cbe8bd59bf))
- **engine:** Add the size watchdog, cancel timeout, limits and bwrap sandbox of the child policy ([3baf0dc](https://github.com/AlexGalhardo/vukem-video/commit/3baf0dc1c90817d86fe77ca63faef079b9df0d92))
- **cli:** Add the command reference for the documentation site ([92c4246](https://github.com/AlexGalhardo/vukem-video/commit/92c424656124e3396579ffd18ebc9fa77e2795ee))
- **site:** Generate the reference pages and add the agents pages ([1e64562](https://github.com/AlexGalhardo/vukem-video/commit/1e64562c14b9b537d1cf5f4110421bc4f42a72aa))
- **site:** Check twins and llms.txt against the built pages ([3c3f1f5](https://github.com/AlexGalhardo/vukem-video/commit/3c3f1f5a51c52ff7e02b6ab536a5c303b8fd8874))
- **ui:** Add the PixiJS preview executor and truth on pause ([f927efd](https://github.com/AlexGalhardo/vukem-video/commit/f927efda89ef9459f7a737bd9da816228d5b3877))
- **engine:** Refuse pictures with unreadable size and add the hostile corpus and fuzz tests ([2fcd1ba](https://github.com/AlexGalhardo/vukem-video/commit/2fcd1ba48d59e5b7303284f2c21f11ea88edfe19))
- **ui:** Add the manual operations, library and export dialog ([bf6b110](https://github.com/AlexGalhardo/vukem-video/commit/bf6b1100387bd37656e4ad8224089d729f055da7))
- **ingest:** Add MinSilence, FramesFromSamples and the audio, silence and loudness tests ([b5d5e8e](https://github.com/AlexGalhardo/vukem-video/commit/b5d5e8e08485bc8d2bdd003c35c45a1bfe6938da))
- **export:** Adapt engine/export.Run to service.Exporter ([0a4062e](https://github.com/AlexGalhardo/vukem-video/commit/0a4062e8afd141ffeea01e3c1f87d48ad3e1b6b0))
- **web:** Run serve as the owner of the workspace ([c92e09b](https://github.com/AlexGalhardo/vukem-video/commit/c92e09bb041ab14d1ca8794b374c3015ae189e83))
- **api:** Mount the MCP handler at POST /mcp behind the guard ([5561de0](https://github.com/AlexGalhardo/vukem-video/commit/5561de0de18e4e5a1bea8ff60b0d68fe9cdf38b9))
- **cli:** Build the in-process Service with the wiring of serve ([4b5f367](https://github.com/AlexGalhardo/vukem-video/commit/4b5f367751d2648d36fd733ed0809ae112279afc))
- **engine:** Index only the cache folders and expose the sheet cache key ([e08bdbb](https://github.com/AlexGalhardo/vukem-video/commit/e08bdbb51eb6ed02da9eefc0ff3a4e84fa5b8f40))
- **service:** Add ViewFrame and ViewSheet over engine/frame and their HTTP routes ([3a18eab](https://github.com/AlexGalhardo/vukem-video/commit/3a18eabb8765793f7e7591724d71d1984c08f4ee))
- **api:** Stream project events with replay from the log and a state snapshot ([61963b8](https://github.com/AlexGalhardo/vukem-video/commit/61963b898d2ec1f3e01d2152ceeb2ece8ae0ce11))
- **service:** Add PlanWindow and serve rasterized bitmaps by key ([20fea25](https://github.com/AlexGalhardo/vukem-video/commit/20fea25fbc346ba9ec9ba9c75669dfbb1bed882f))
- **ui:** Add check:i18n, the pt-BR catalogue and the coverage declaration ([ee1d2d7](https://github.com/AlexGalhardo/vukem-video/commit/ee1d2d7689f9206ddafa6bd61858586f56132991))
- **site:** Serve the docs in English and Brazilian Portuguese ([c635d09](https://github.com/AlexGalhardo/vukem-video/commit/c635d09e3c1169e49164a2895b2139578e1b4a87))
- **site:** Check translations and the locale properties of the build ([9b7de37](https://github.com/AlexGalhardo/vukem-video/commit/9b7de37e302d90ceb6a4df94b6ef0d905e9c7b6f))
- **engine:** Add cache leases and the pruning of old frame revisions ([da6ed3d](https://github.com/AlexGalhardo/vukem-video/commit/da6ed3dc49c1a27f594078cce2080984bf2a745c))
- **mcp:** Serve the view tool and the view command over ViewFrame and ViewSheet ([ac5b716](https://github.com/AlexGalhardo/vukem-video/commit/ac5b7160d6ce262473afba745a0ea96dfb6d2f91))
- **cli:** Add the events command over the server event stream ([b88b16c](https://github.com/AlexGalhardo/vukem-video/commit/b88b16ce7744191a88a0e5825724b24b96269493))
- **web:** Serve the projects page, the editor page and the static files ([e58f98d](https://github.com/AlexGalhardo/vukem-video/commit/e58f98d36101b39f0228ec34d1d735246b45642f))

### Fixed

- **service:** Render the revision pinned when an export starts ([532f700](https://github.com/AlexGalhardo/vukem-video/commit/532f7000f0351400f1afdc3a2d2736dd603569f7))
- **service:** Wait for the asset.add before a waiting import returns ([56a2d9e](https://github.com/AlexGalhardo/vukem-video/commit/56a2d9ee353af686c66f175e349beb893b206ff8))
- **service:** Carry the new state in project.updated ([730cdbf](https://github.com/AlexGalhardo/vukem-video/commit/730cdbf35a92e534777a5daa00c1b07808174b42))
- **ui:** Run check:i18n with Node, whose child pipes the TypeScript API needs ([e802e99](https://github.com/AlexGalhardo/vukem-video/commit/e802e99c7a1d3978a76ccc737c95154bc120ecb7))
- **site:** Fill the Required column of the generated argument tables ([6c0d1c3](https://github.com/AlexGalhardo/vukem-video/commit/6c0d1c3d0027db0e695d3a3a221615f3a4cd0a18))
- **repo:** Address the CodeQL and Linux lint findings of the v0.4.0 pull request ([c5ddfd2](https://github.com/AlexGalhardo/vukem-video/commit/c5ddfd2cf13f63b8ef78e6c69875ed1f9ee090c1))
- **service:** Open imports by path components and read free space per OS ([b47ff27](https://github.com/AlexGalhardo/vukem-video/commit/b47ff27919189a7be9eaa23908902f33647e03ea))

### Security

- **security:** Scope remembered bitmap keys to the project that planned them ([3b7a631](https://github.com/AlexGalhardo/vukem-video/commit/3b7a631ac15747113d7bb6c6d12b87b06245f1d5))

## [0.3.0] - 2026-11-15

### Added

- **ui:** Add the editor API client, edit queue and event stream ([113a1a9](https://github.com/AlexGalhardo/vukem-video/commit/113a1a9aba1eb9e7cca47401bf2c4188fbe1b59e))
- **ui:** Add the editor read model and its synchronization state machine ([93f411a](https://github.com/AlexGalhardo/vukem-video/commit/93f411a0cbdda120326d1c6cb7092c550177e0b0))
- **ui:** Add the editor entry, shell, top bar and timeline ([a905c27](https://github.com/AlexGalhardo/vukem-video/commit/a905c27ec7f98d2231f3c9ee9243d864d0caef82))
- **ui:** Add check-thin-client with rules R1 to R8 and its fixtures ([047549b](https://github.com/AlexGalhardo/vukem-video/commit/047549bde76fe01d9a74e38769a4f2b79d2623e4))
- **service:** Add the error, the caller and the request types ([e3561e1](https://github.com/AlexGalhardo/vukem-video/commit/e3561e198a2be7a3633314814aa80e64fc6f859f))
- **service:** Render the project views, lists and history as text ([db831e6](https://github.com/AlexGalhardo/vukem-video/commit/db831e6e8db5adccf1bf410e0ab9b178a767f236))
- **lint:** Page the report by a limit and leave room for the cursor line ([d016c05](https://github.com/AlexGalhardo/vukem-video/commit/d016c05efb12e858c2e63071ff51ce1e69b31cec))
- **service:** Add the crash points of the commit path ([1128347](https://github.com/AlexGalhardo/vukem-video/commit/11283479673560aa8e55cdeb76565e4cf86fa633))
- **service:** Add Service with projects, the commit transaction, history, undo, redo and lint ([86fe0ab](https://github.com/AlexGalhardo/vukem-video/commit/86fe0aba6619c5fffe44cb576da15270de8f3596))
- **service:** Add EditInternal and test that project_assets and the summary follow the document ([2b2e752](https://github.com/AlexGalhardo/vukem-video/commit/2b2e752bc5de26a7b4fe622aa559fa7cdb475fec))
- **service:** Add CommandHelp over the public commands of the registry ([4d3e6a2](https://github.com/AlexGalhardo/vukem-video/commit/4d3e6a2ee6b3a00d06807953ccb23d3a5f5064d5))
- **web:** Add the local-mode credentials: launch codes, sessions, actor label and handshake ([1e892de](https://github.com/AlexGalhardo/vukem-video/commit/1e892def1c5e0a2fcaa488b66eccbdeaa7d7a441))
- **web:** Add the loopback client and make the handshake client a Doer ([adea75f](https://github.com/AlexGalhardo/vukem-video/commit/adea75f476c49583712d4c7b6a924a8f78ec8b3e))
- **api:** Add the HTTP adapter with the error envelope, local-mode trust and OpenAPI ([8bd3308](https://github.com/AlexGalhardo/vukem-video/commit/8bd3308e98d7bcf24af032feb19d6a9b0197c1ce))
- **cli:** Add the command framework, the project, edit, history, undo, redo and lint commands ([b3d46b2](https://github.com/AlexGalhardo/vukem-video/commit/b3d46b290af78c579d62647ad31bc22430d1fff3))
- **cli:** Resolve the Service in process over the workspace store ([a2d7062](https://github.com/AlexGalhardo/vukem-video/commit/a2d7062c65cfbcac033effe10464d301da943760))
- **repo:** Add the third-party notices generator ([3f4bb3e](https://github.com/AlexGalhardo/vukem-video/commit/3f4bb3ef83cad96b084d3f91e32346b017442ee8))
- **timeline:** Move the migration chain to its own package with frozen version fixtures ([66c12c5](https://github.com/AlexGalhardo/vukem-video/commit/66c12c5b8ead6318560d73a287d36feb2e8f1b9d))
- **service:** Add the HTTP client of the Service ([7dbf3eb](https://github.com/AlexGalhardo/vukem-video/commit/7dbf3eb3f3dc3e63e243941d4f6ce239eb86d2fe))
- **cli:** Reach a running owner over HTTP and add command:help ([5cb8842](https://github.com/AlexGalhardo/vukem-video/commit/5cb8842d2e15e5a5f24b298701c64d5971be7786))
- **cli:** Run the product commands from the binary ([20b99aa](https://github.com/AlexGalhardo/vukem-video/commit/20b99aae4493ed407e85946c7c6eebba5d2f86a8))
- **engine:** Feed standard input to FFmpeg and allow atempo ([ef1a3fd](https://github.com/AlexGalhardo/vukem-video/commit/ef1a3fdf9d753a5e08426c4acb5796fe5eafdf30))
- **engine:** Add the Go compositor ([0d9cadd](https://github.com/AlexGalhardo/vukem-video/commit/0d9cadd8a7b15792f6c970463bbfa9cb1e019a95))
- **engine:** Add the audio mixer and the loudness gain ([6bbbd4d](https://github.com/AlexGalhardo/vukem-video/commit/6bbbd4d93ed428d9378fcc660feed7e7a69e0593))
- **engine:** Add a box-filtered resize to the compositor ([05ead60](https://github.com/AlexGalhardo/vukem-video/commit/05ead60b03ff1a15e57aa813cecd735962c7fe15))
- **export:** Export a project to a verified MP4 ([391e9a9](https://github.com/AlexGalhardo/vukem-video/commit/391e9a9d8c57c9f043a2c4361e9ecb73f884bd80))
- **cli:** Add the version command ([5d38271](https://github.com/AlexGalhardo/vukem-video/commit/5d38271f211a4057735a0d4d98905bb74ec84463))
- **plan:** Add snap points and the output scale of the oversampling, pin the ALPHA plan asserts ([b9f7398](https://github.com/AlexGalhardo/vukem-video/commit/b9f7398d93c12d2fe279f1fb8ce9b7bde3ea890f))
- **engine:** Add the byte-capped file cache for frames and bitmaps ([69d7b83](https://github.com/AlexGalhardo/vukem-video/commit/69d7b83d24848556bc0111f9a100c6a86ec32994))
- **export:** Add frames and contact sheets from the export compositor ([778924f](https://github.com/AlexGalhardo/vukem-video/commit/778924f0fe87c3fd86b486e85268865a9ab0482f))
- **repo:** Read the whisper.cpp version of the notices from its pin ([e91f5ab](https://github.com/AlexGalhardo/vukem-video/commit/e91f5ab693d0dc9815f37c9652733ee6f349fd1b))
- **command:** Add settings.set_fps and asset.remove with their golden cases ([77f07fc](https://github.com/AlexGalhardo/vukem-video/commit/77f07fcd1027f7352baf4b58f5a0915aef23c86b))

### Fixed

- **ci:** Read the Docker Hub user from the repository variable ([70f60f5](https://github.com/AlexGalhardo/vukem-video/commit/70f60f5392c1eaa2da558ca786d958a39bc54f38))
- **service:** Scope the project list, its total and creation to the caller ([4ae3ae7](https://github.com/AlexGalhardo/vukem-video/commit/4ae3ae7bab066bc4cedcb2149d130cdf20ac0554))
- **service:** Purge only the committing project's expired idempotency keys ([6790828](https://github.com/AlexGalhardo/vukem-video/commit/6790828df01a558637f12a1853c83d2d90f9e21a))
- **repo:** Put bare e-mail addresses of the notices in angle brackets ([4c512fe](https://github.com/AlexGalhardo/vukem-video/commit/4c512fef64aabefa79bf1b31ff669402355d1b3e))
- **ui:** Start the e2e server with the serve command ([8f4cc9c](https://github.com/AlexGalhardo/vukem-video/commit/8f4cc9ce4b83ff9ca1fda08dc4acddbb906bb271))
- **engine:** Fix the filter and encoder thread counts so frames do not depend on the CPU count ([600b71a](https://github.com/AlexGalhardo/vukem-video/commit/600b71a4593a12a1fb5a1cd752da7854abfd1817))
- **engine:** Encode the normalized mpeg4 with the integer DCT so it does not depend on the CPU ([2c74861](https://github.com/AlexGalhardo/vukem-video/commit/2c74861054fdc3c688d0365b18c2910f0a7e2e44))

### Security

- **security:** Allow example actor ids in Go tests and the editor ([a9f25a0](https://github.com/AlexGalhardo/vukem-video/commit/a9f25a0827a75621ff2f2adfb0de94f89770c711))

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

[Unreleased]: https://github.com/AlexGalhardo/vukem-video/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/AlexGalhardo/vukem-video/releases/tag/v0.1.0
