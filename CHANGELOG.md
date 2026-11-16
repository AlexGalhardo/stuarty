# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.17.0] - 2026-11-16

### Added

- **ui:** Send an ended session on the keys page to the login ([371dc2d](https://github.com/AlexGalhardo/vukem-video/commit/371dc2d6d6faa4b21d0ef26f007cf7a3a703843b))

### Fixed

- **engine:** Plan each frame of a Renderer at its output scale ([376c26f](https://github.com/AlexGalhardo/vukem-video/commit/376c26fe89276153bf5bd6a649f1f5eadc66e6ae))
- **engine:** Centre a text bitmap on its block when a shadow reaches further on one side ([39915de](https://github.com/AlexGalhardo/vukem-video/commit/39915de276aa294ba5a5b490df8bec27f1a3a0fe))
- **engine:** Crop a clip's colour at an odd corner on the plan rectangle ([2d5edf5](https://github.com/AlexGalhardo/vukem-video/commit/2d5edf5fc9d6f9664f1571d388ed87ceea79738d))
- **ui:** Send a JSON content type on every call that changes state ([dbd01fe](https://github.com/AlexGalhardo/vukem-video/commit/dbd01fe20d016dc965c0f4e6069553d1ba5d44ae))
- **ui:** Size the preview buffer like ViewFrame and compare the buffer pixels ([8df7166](https://github.com/AlexGalhardo/vukem-video/commit/8df7166d5521ac20086aad71b1f1fb5d4714b561))
- **ci:** Pin the OpenH264 headers by commit and digest ([10560fc](https://github.com/AlexGalhardo/vukem-video/commit/10560fc5b3e992945a94354262964e38de25b945))
- **ci:** Expand empty arrays safely for bash 3.2 in the FFmpeg build ([cf3dc1f](https://github.com/AlexGalhardo/vukem-video/commit/cf3dc1fc0fa4e12890b95e98b9eaf3bd952547ec))
- **ci:** Link libm for zimg and use the host pkg-config when cross compiling ([51f488d](https://github.com/AlexGalhardo/vukem-video/commit/51f488db8a2fffaec881a4371a047ea9879e0e53))
- **ci:** Build the windows encoders and enable NVENC under disable-autodetect ([49b0a99](https://github.com/AlexGalhardo/vukem-video/commit/49b0a99d9dbfc1ac3f90a1d04174c1ab9d55a46a))
- **ci:** Stop the encoder checks of ffmpeg-verify from dying of SIGPIPE ([87ca392](https://github.com/AlexGalhardo/vukem-video/commit/87ca392b7936d17b8519f056f47944455936e001))

### Security

- **security:** Add the no-egress job with scripts/no-egress.sh ([046f379](https://github.com/AlexGalhardo/vukem-video/commit/046f379798cf6416a50866081be9791acd962f08))
- **security:** Add the dependency-review job ([eb982f8](https://github.com/AlexGalhardo/vukem-video/commit/eb982f8297f60662c17d434cd2933e44e9f82cf5))
- **security:** Fuzz the path, untrusted text, OTIO and host targets ([fc8c8b7](https://github.com/AlexGalhardo/vukem-video/commit/fc8c8b760323bea0b0954f65b9dffee9f5da516b))
- **security:** Allow the pinned digests and e2e passwords and satisfy the linters ([a8e57ed](https://github.com/AlexGalhardo/vukem-video/commit/a8e57edc16f50423a0bf4cad5322f668f85fdc59))

## [0.16.0] - 2026-11-16

### Added

- **release:** Sign the desktop update feed with an Ed25519 key ([8768cfb](https://github.com/AlexGalhardo/vukem-video/commit/8768cfb3cd71f4b30a220325f19a4e11fbe9c6c6))
- **release:** Add desktop-verify.sh for the unsigned desktop installers ([0545a1e](https://github.com/AlexGalhardo/vukem-video/commit/0545a1e6bcc4058adc7a8c35a9849bcd5ca12ebc))
- **release:** Require a major bump for a breaking change after 1.0 ([aeb7914](https://github.com/AlexGalhardo/vukem-video/commit/aeb7914ef7a2f48732fede992076846cd109d297))
- **site:** Add a link check over the deployed site ([f326487](https://github.com/AlexGalhardo/vukem-video/commit/f32648729fdfadfdf59d2d87f34c32256b060e71))
- **release:** Add the upgrade path test script across released versions ([944f3a6](https://github.com/AlexGalhardo/vukem-video/commit/944f3a6c47f70ba3e008db0a64cb9e050a86cef6))
- **release:** Add licence audit modes to the notices tool ([eb9f6fb](https://github.com/AlexGalhardo/vukem-video/commit/eb9f6fb400c7f0c8c21f56d8c961f19abb178d92))
- **release:** Audit SBOM waivers, innoextract for Inno installers and an audit job ([f3c2912](https://github.com/AlexGalhardo/vukem-video/commit/f3c2912ee9035b758c867434918ec4662ef0f1d2))
- **engine:** Fall back to the next encoder when the chosen one cannot do the export size ([26fb853](https://github.com/AlexGalhardo/vukem-video/commit/26fb853fb74d40b27c2762573a60d5b4984e72c8))
- **site:** Check that pt-BR pages keep the headings of their English page ([42ac4c8](https://github.com/AlexGalhardo/vukem-video/commit/42ac4c84b7fbea48b76ef6d2528f3316f98db0f5))
- **site:** Smoke-test the tutorial commands on a local server ([e711ccf](https://github.com/AlexGalhardo/vukem-video/commit/e711ccf400bcdc7e4085400022d02a33f47fdfbb))
- **release:** Add the bundle matrix script and its hash tool ([989bd63](https://github.com/AlexGalhardo/vukem-video/commit/989bd63c78bcb52adaa9a686c6562aa17411f5d7))
- **web:** Serve the login and API keys pages in network mode ([da6356d](https://github.com/AlexGalhardo/vukem-video/commit/da6356d7f1954ebfecdcc0b1bfcdfaa8d01885c5))

### Fixed

- **engine:** Accept an MP4 whose first bytes read as text when its ftyp box is well formed ([9ee30b6](https://github.com/AlexGalhardo/vukem-video/commit/9ee30b60b08d2341d1add3532c156b94183e2e2b))
- **ui:** Give the select viewport and the Inertia pages the CSP nonce for their style elements ([35fbd7c](https://github.com/AlexGalhardo/vukem-video/commit/35fbd7cd2795783a851e7aed8cf57c74e546ff8d))
- **web:** Show the newer-workspace and loose-token reasons on every surface ([57f1665](https://github.com/AlexGalhardo/vukem-video/commit/57f1665c6bedc3fb3b339f4a243913288c8d30bd))
- **ui:** Show the code, message and hint when the editor cannot load a project ([1cf396d](https://github.com/AlexGalhardo/vukem-video/commit/1cf396d1282c06c8e4de090aeef0421cb60c6519))
- **web:** Map an unusable workspace path to E_INVALID_STATE without naming it ([e43fe67](https://github.com/AlexGalhardo/vukem-video/commit/e43fe679ea27175105c41637682aec116e8740cc))
- **service:** Sweep bundle export temp files that a killed process left ([421d218](https://github.com/AlexGalhardo/vukem-video/commit/421d218442ae02604af24d99e7a82e0b65d8be4b))
- **service:** Refuse a stored document of a newer schema version with V_SCHEMA_VERSION ([b12fae8](https://github.com/AlexGalhardo/vukem-video/commit/b12fae887231c669658aeaa6611f5e6743ffa72c))
- **web:** Show the code, message and hint when the editor page cannot open a project ([7550b7e](https://github.com/AlexGalhardo/vukem-video/commit/7550b7eb430ef17dbd4e7aae423e1ab63f3ed562))
- **installers:** Carry the third-party notices and licenses in every installer ([ff0f5af](https://github.com/AlexGalhardo/vukem-video/commit/ff0f5afe09ada795a49fc6b126de5e3430b8a43a))
- **release:** Expect 21 assets and skip the desktop and feed checks before v0.16.0 ([cdfb5e4](https://github.com/AlexGalhardo/vukem-video/commit/cdfb5e4ccb7ab5c23036a4539df1fcd8ed066fa6))
- **plan:** Skip the placeholder items of an unfilled optional slot in the audio sources ([7db0b33](https://github.com/AlexGalhardo/vukem-video/commit/7db0b33ed1306a9dc45bfeab090ed60ae9e2c843))
- **lint:** Leave an optional slot on its placeholder quiet so the starters export ([6cd0c43](https://github.com/AlexGalhardo/vukem-video/commit/6cd0c4394c3dd385679c4a802bf0f8a89f718e74))

### Security

- **security:** Count T-35 in the threat model ([7b41c23](https://github.com/AlexGalhardo/vukem-video/commit/7b41c23a265b1a859300b9024c71a1d9b6709d78))

## [0.15.0] - 2026-11-15

### Added

- **ui:** Extend i18n rule I3 to all of resources/js ([edb3955](https://github.com/AlexGalhardo/vukem-video/commit/edb39556493293b22c70950896cea0fcdc59be6e))
- **service:** Tag the OpenTimelineIO result names as untrusted text ([814aaf0](https://github.com/AlexGalhardo/vukem-video/commit/814aaf023abb30d51bee83f2d37b6fe1c7af1071))
- **service:** Install the Cisco OpenH264 library through the explicit install step ([0fcf602](https://github.com/AlexGalhardo/vukem-video/commit/0fcf60229fa4cca5ba089c5bb0d624b6e4bfb732))
- **service:** Show the build flavour in doctor and check the OpenH264 library of the own build ([94ba796](https://github.com/AlexGalhardo/vukem-video/commit/94ba796b06eb5a547ff850a5b7b0b97b795b3984))
- **ui:** Wire the update client into the desktop shell menu and settings ([bd92aed](https://github.com/AlexGalhardo/vukem-video/commit/bd92aed9d19e31394533421a262b68e4d46e1d7b))
- **ui:** Ask the plan for the preview buffer width ([4fd14d7](https://github.com/AlexGalhardo/vukem-video/commit/4fd14d7b55af5e2a16f042b13dbf573b9018d023))
- **engine:** Apply the Job Object limits to FFmpeg and whisper children on Windows ([e2f1fba](https://github.com/AlexGalhardo/vukem-video/commit/e2f1fbaad479d7f5dc0c1366c085499a2ced45a1))

### Fixed

- **transcribe:** Refuse a relative helper or model path ([afcb70f](https://github.com/AlexGalhardo/vukem-video/commit/afcb70ff752cf5d29fae669c17d7c1541ae014ed))
- **engine:** Give the bwrap probe a minimal environment ([abaca59](https://github.com/AlexGalhardo/vukem-video/commit/abaca598e13c3e0bc3923fb55f88180f830e6b1c))
- **site:** Treat opentimelineio.org as an inert link host ([4789e58](https://github.com/AlexGalhardo/vukem-video/commit/4789e58506c4f8a87226647f508197682c68bd56))
- **ui:** Keep the rotation handle reachable under the toolbar ([e11d29d](https://github.com/AlexGalhardo/vukem-video/commit/e11d29d79a8821fea02b3f152db08a8d2092b4ba))
- **ui:** Correct pt-BR terms to the glossary ([e4b2739](https://github.com/AlexGalhardo/vukem-video/commit/e4b27398c1fe3349931de4bd1e59bd229cfe9191))
- **ui:** Translate the close button of dialogs and sheets ([7866708](https://github.com/AlexGalhardo/vukem-video/commit/7866708224ba9813aa38d0a96298260b4a4c8651))
- **ui:** Use the glossary term for playhead in pt-BR ([644bb82](https://github.com/AlexGalhardo/vukem-video/commit/644bb8225c78aa2ad6e86a39377c0f7209fbd4ad))
- **ui:** Translate the sidebar strings ([d59ec17](https://github.com/AlexGalhardo/vukem-video/commit/d59ec17c38cd07537464a3d96763fbc94453aaa5))
- **service:** Carry the limit, the value and the ceiling in every limit error ([de4bf2b](https://github.com/AlexGalhardo/vukem-video/commit/de4bf2b31626214b41505dfca573ef493ef10dd1))
- **web:** Carry the limit, the value and the ceiling in stream, frame and request refusals ([6845e20](https://github.com/AlexGalhardo/vukem-video/commit/6845e206089649137d87f11ae19d6cc0c51a29fa))
- **service:** Refuse a new idempotency key at the ceiling instead of evicting the oldest ([2ec933e](https://github.com/AlexGalhardo/vukem-video/commit/2ec933e2d0689f9bb21a635eda3d6fdc34bbe35e))
- **service:** End an export over the byte ceiling with E_EXPORT_FAILED and reason size_limit ([63b0b34](https://github.com/AlexGalhardo/vukem-video/commit/63b0b34ba7e0063da098eb27efb4ac1bddde0a0d))
- **service:** Answer E_QUEUE_FULL for a file of an import that finds the ingest queue full ([48710fc](https://github.com/AlexGalhardo/vukem-video/commit/48710fc89118f834026ad12a41db345b214d6ca9))
- **cli:** Print the cleaned names of the OpenTimelineIO export ([0d76a37](https://github.com/AlexGalhardo/vukem-video/commit/0d76a3703db3ebc85155a8f75b16e627c862621a))
- **ui:** Keep the preview canvas when the browser has no Web Audio ([f7a217a](https://github.com/AlexGalhardo/vukem-video/commit/f7a217a7b3a5082aceef5825966a525623155586))
- **ui:** Let the brand font button name contain its visible text ([f35ce72](https://github.com/AlexGalhardo/vukem-video/commit/f35ce72c9927df84f09c7e97c6c18f8d98f88ac4))
- **command:** List E_NOT_FOUND for font.add, raised by the service before apply ([bb51260](https://github.com/AlexGalhardo/vukem-video/commit/bb512606f31a1569462aa037b2b068ca1d2e2def))

### Security

- **security:** Pin the licence allowlist and licence texts of shipped components ([7f9a510](https://github.com/AlexGalhardo/vukem-video/commit/7f9a5103a51ca930541884afb18a40ad4957e725))
- **security:** Refuse a relative server path and strip the bwrap probe environment ([a469ff3](https://github.com/AlexGalhardo/vukem-video/commit/a469ff386986423980edeccd7c6ef2575a9c704b))
- **security:** Add the desktop update outbound purpose ([243b376](https://github.com/AlexGalhardo/vukem-video/commit/243b376532d32070af7a6c830c5e3ebf6bbfa7c2))
- **security:** Add the desktop update client with signed feed and artifact checks ([9b6fe6d](https://github.com/AlexGalhardo/vukem-video/commit/9b6fe6d00c08a19967ca976a16390bc08767d6f2))

## [0.14.0] - 2026-11-15

### Added

- **service:** Add the tables and repositories of font owners and the brand kit ([cfd9af2](https://github.com/AlexGalhardo/vukem-video/commit/cfd9af2bb8ae4e1344192f3c3fc7062a91f71ac1))
- **service:** Import fonts, keep a brand kit and record who owns each font ([d8bc15b](https://github.com/AlexGalhardo/vukem-video/commit/d8bc15b0c21a7798f35c62def6d0594da6ba05d7))
- **web:** Route the font import and the brand kit ([178fa21](https://github.com/AlexGalhardo/vukem-video/commit/178fa21ea37b04ac7cd462836e6959679c7f4ece))
- **service:** Carry user fonts in template files ([1e3f858](https://github.com/AlexGalhardo/vukem-video/commit/1e3f85852ee5a3d27def841e84b9534af11b6aa9))
- **ingest:** Allow the animated image demuxers and probe frame delays within ceilings ([c6b7ef4](https://github.com/AlexGalhardo/vukem-video/commit/c6b7ef4cef13f1dbefbb1aba31827a8cb59acd8e))
- **ingest:** Decode animated images to a frame sequence artifact with their delays ([613924a](https://github.com/AlexGalhardo/vukem-video/commit/613924a2d029911f4840027c7e48c025fa962817))
- **export:** Draw the frame of an animated image that the plan names ([97474d5](https://github.com/AlexGalhardo/vukem-video/commit/97474d55b5e3ca5f367a860e5aa2096f19f33f99))
- **service:** Keep the frame delays of an animated image and its frames artifact ([9c42e8d](https://github.com/AlexGalhardo/vukem-video/commit/9c42e8dad34bbf48f7556b926c26bc144e74f36d))
- **engine:** Test the chosen encoder at the size and rate of the export ([13b20c1](https://github.com/AlexGalhardo/vukem-video/commit/13b20c1f4513d0c769965da558bb19f2b536a88e))
- **export:** Gif, audio only, 60 fps and 4K exports with verification ([6fb9033](https://github.com/AlexGalhardo/vukem-video/commit/6fb90337ece16ebcb83a52fd1617a3954220e78f))
- **ui:** Resize track rows with a splitter and keep the heights in vv.layout ([f5596fc](https://github.com/AlexGalhardo/vukem-video/commit/f5596fcbd6c23793690eab35e06aff6f4a6dbbfc))
- **ui:** Add preview size, volume, mute and full screen to the transport row ([6070749](https://github.com/AlexGalhardo/vukem-video/commit/6070749318d846771be7631443bb619e05dcaa3f))
- **ui:** Show the names of network users in the activity indicator and history ([8343a25](https://github.com/AlexGalhardo/vukem-video/commit/8343a253d2e3fa0e267bf30917a6f14f11a01908))
- **ui:** Fade state changes and ease in highlights, off under reduced motion ([021ace5](https://github.com/AlexGalhardo/vukem-video/commit/021ace50cec7d901c6dc874c53f85a25058ac3ad))
- **cli:** Add font:import, brand:show and brand:set and the template:new brand flags ([0dfbbb9](https://github.com/AlexGalhardo/vukem-video/commit/0dfbbb9a7d1166198d0a4bb0baef489097485a9c))
- **ui:** Add brand kit, font import, bundle and export option calls to the API client ([fb5dc74](https://github.com/AlexGalhardo/vukem-video/commit/fb5dc744fd96d2e6166f9e4a8009c6a8e7d8bad7))
- **ui:** Add the brand kit page with colours, fonts and font import by path ([5d747c8](https://github.com/AlexGalhardo/vukem-video/commit/5d747c839578d5d4c08df4f4051b87de9e75e6fb))
- **ui:** Offer brand kit colours and fonts in the inspector and in new projects from a template ([98a5309](https://github.com/AlexGalhardo/vukem-video/commit/98a53094ad7c2437d9051efd0a4b99b924ed95f0))
- **ui:** Add format, frame rate and size options to the export dialog from the server schema ([09af9aa](https://github.com/AlexGalhardo/vukem-video/commit/09af9aab7842f877442bb9211eb9af2c8e3e98b7))
- **ui:** Export a project as a bundle and import a bundle by path from the projects page ([9744729](https://github.com/AlexGalhardo/vukem-video/commit/97447295215405724187b6d0cfa58b5f39743af0))
- **service:** Export gif, wav and aac through JobStart with the format argument ([b64655a](https://github.com/AlexGalhardo/vukem-video/commit/b64655aad3a62373c0296bcd582f381b297ef324))
- **service:** Export and import OpenTimelineIO through the service ([484494f](https://github.com/AlexGalhardo/vukem-video/commit/484494fff935c63d90534ff4c5d5d697fcf585fe))
- **service:** Rasterize plan window bitmaps at the output width ([cfb4e59](https://github.com/AlexGalhardo/vukem-video/commit/cfb4e59c866af4c2e4c6d050808d368e721a74c1))
- **web:** List the job start arguments with the export formats in the OpenAPI document ([1cc2e09](https://github.com/AlexGalhardo/vukem-video/commit/1cc2e091405fda10ff87df2bd85f89a9bed23570))
- **engine:** Recognise the own LGPL build and the BtbN build from the configure line ([8bae736](https://github.com/AlexGalhardo/vukem-video/commit/8bae736dc268c517de2dfd8690352a8cd7e887b9))
- **engine:** Pin and install the Cisco OpenH264 library for the own build with checksums ([2a47063](https://github.com/AlexGalhardo/vukem-video/commit/2a4706311dc1b18b7032c0a7c8559e3d0e7fe5d0))
- **engine:** Gate libopenh264 on the installed Cisco library and hand it to the own build ([08e3170](https://github.com/AlexGalhardo/vukem-video/commit/08e3170bf030766db2c646f940562a3f0ac5e647))
- **mcp:** List the export format in the job_start schema ([ee0bade](https://github.com/AlexGalhardo/vukem-video/commit/ee0bade7a0539c2b2c6529e75cbc375bb5bbbf21))
- **cli:** Add the export format flag ([8d222c8](https://github.com/AlexGalhardo/vukem-video/commit/8d222c85486632e443772377a53edbb91e61397f))
- **cli:** Add otio:export and otio:import ([be130d4](https://github.com/AlexGalhardo/vukem-video/commit/be130d4c630e637978a256bc613858d59970e80b))

### Fixed

- **ingest:** Cap the animated image decode at the probed frame count and its declared size ([125ad86](https://github.com/AlexGalhardo/vukem-video/commit/125ad867934c3a2b38e3b43fdbdaf695b21762dc))
- **engine:** Remove an oversized partial output only inside the run directory ([de8e00a](https://github.com/AlexGalhardo/vukem-video/commit/de8e00a0f52aa4af5d84a945e2faa5e16dd22bf4))
- **ui:** Size the preview renderer once its async init finishes ([77d986f](https://github.com/AlexGalhardo/vukem-video/commit/77d986f76d19ad82a72bf8f140f69e1e3d9aafc9))
- **ui:** Draw the colour matrix as 5x4, the filter matrix and the blur in the preview ([7aee308](https://github.com/AlexGalhardo/vukem-video/commit/7aee308f5403c828f91d5facff5c6ad9d43de0f6))
- **engine:** Crop video layers at the exact plan rectangle on subsampled sources ([58862dd](https://github.com/AlexGalhardo/vukem-video/commit/58862ddceae5a9a0c312968a3f0be077bc449a4d))
- **ui:** Wrap the header and project rows at 320 px and add a projects h1 ([75399db](https://github.com/AlexGalhardo/vukem-video/commit/75399db295e7d21a4523918d8aeaa85eafe3767f))
- **ui:** Offer only the export format, since frame rate and size are project settings ([f5f7c92](https://github.com/AlexGalhardo/vukem-video/commit/f5f7c929d94a8bbce6d4b8398aa32dfe85dc109a))

### Security

- **security:** Name the font ownership tests and the bundle operations in the scope matrix ([c2c7259](https://github.com/AlexGalhardo/vukem-video/commit/c2c7259b2894dd1a4c40aa8e735d774ab259099f))
- **security:** Drop the BETA allowlist of threatcheck now that T-21 has its tests ([a1aaf04](https://github.com/AlexGalhardo/vukem-video/commit/a1aaf0428216be73458b86bdc9635197f5ee66fa))
- **security:** Cover bundle declared sizes and crafted or unshipped fonts in a bundle ([173a67d](https://github.com/AlexGalhardo/vukem-video/commit/173a67d87e12ac7f6a54da5c9ec4de95397b6091))
- **security:** Seed the archive fuzzer with bundle shaped archives ([2965089](https://github.com/AlexGalhardo/vukem-video/commit/29650892be06cceeabb86fa826edefb86d722833))
- **security:** Fuzz the template and bundle archive readers weekly ([6af3b01](https://github.com/AlexGalhardo/vukem-video/commit/6af3b0139addcd655ccd34d3f9e411cbd46d6a14))
- **security:** Revalidate the OpenH264 manifest entry and the answering host at install ([52f86aa](https://github.com/AlexGalhardo/vukem-video/commit/52f86aa45ebe33b43aae22ea38b87a89461585df))
- **security:** Cover own build flags, library directory and swap after install ([d3fc737](https://github.com/AlexGalhardo/vukem-video/commit/d3fc737839e1ccc94aad3c51a43314b14da60773))
- **security:** Satisfy the linters in the supply-chain tests ([c9a2aeb](https://github.com/AlexGalhardo/vukem-video/commit/c9a2aeb6617498be6849b2ffe22f1e79f8d0ba14))
- **cli:** Never trust the file name the server returns for otio:export ([ad1ae4b](https://github.com/AlexGalhardo/vukem-video/commit/ad1ae4b042ad74d7d3886cd29b3a47ffddac02e4))
- **security:** List the brand kit page and allow the password label ([204bfb5](https://github.com/AlexGalhardo/vukem-video/commit/204bfb592861d46b7a1ec2cc5cb68d7b419d557e))

## [0.13.0] - 2026-11-15

### Added

- **ingest:** Speech map and beat grid artifacts computed in Go ([dbffd6b](https://github.com/AlexGalhardo/vukem-video/commit/dbffd6b2a295a254c753765d0f31dad9d6ce52d3))
- **export:** Blur and filter matrices of a layer in Go ([a63a363](https://github.com/AlexGalhardo/vukem-video/commit/a63a363b928be5e92678936fe6c232cdb988e27d))
- **ingest:** Store the speech map and the beat grid of every ingest job ([e1e06d5](https://github.com/AlexGalhardo/vukem-video/commit/e1e06d52f5d424014d8f10110827a80e0239f4de))
- **cli:** Add the transcript and silence recipes and tie every recipe to a contract scenario ([b0d55da](https://github.com/AlexGalhardo/vukem-video/commit/b0d55daae5bff398f4f5f8741f52aee1e15e9dae))
- **command:** Add audio.duck and audio.mark_beats and keep markers through edits ([7e51c13](https://github.com/AlexGalhardo/vukem-video/commit/7e51c134202da2ddb6aee060f767712050f5e172))
- **plan:** Pick the frame of an animated image from its delays ([5001982](https://github.com/AlexGalhardo/vukem-video/commit/500198250acdfee4bbb5e053b79e4bcba004fe8e))
- **command:** Add font.add and font.remove for project fonts ([bb9eccb](https://github.com/AlexGalhardo/vukem-video/commit/bb9eccbc80f98ff778f8c061409aacefb49855f9))
- **ui:** Define window.vukemDesktop in the desktop shell and fix its menu ([9059459](https://github.com/AlexGalhardo/vukem-video/commit/9059459898a9f022e124f46c434673e9f2e3dc03))
- **ui:** Choose files and drop paths in the library through the desktop bridge ([8afc8e7](https://github.com/AlexGalhardo/vukem-video/commit/8afc8e7b6dae21c8b8ddff74830c637087bf6b94))
- **service:** Import a project bundle with the archive policy and the shared store rule ([06c7858](https://github.com/AlexGalhardo/vukem-video/commit/06c7858bb87fb5fd4eba25571c31288848b50644))
- **web:** Route user management for an administrator session ([0e4f683](https://github.com/AlexGalhardo/vukem-video/commit/0e4f6837ee9b1946790f8cb3fb158cf34793137a))
- **cli:** Add bundle:export to write a .vvbundle of a project ([69ef192](https://github.com/AlexGalhardo/vukem-video/commit/69ef19215fa162aefea1e23fa9adf124ee5dd448))
- **ui:** Add builders and coverage entries for the BETA item commands ([21399bb](https://github.com/AlexGalhardo/vukem-video/commit/21399bbe8a549fda38c4732e467ed84a70ef29f1))
- **ui:** Add inspector sections for filters, ducking, beat markers and fonts ([7df17b8](https://github.com/AlexGalhardo/vukem-video/commit/7df17b8a63a11f7bdd7d87f2a1e386d628bc9184))
- **ui:** Announce edit results and errors in a live region ([6bef964](https://github.com/AlexGalhardo/vukem-video/commit/6bef964d00ac898b9cba442df59a8f20497f2115))
- **ui:** Add F6 region jumps, select all and the shortcut table of the keyboard map ([0de84ab](https://github.com/AlexGalhardo/vukem-video/commit/0de84abab179e18947d46bbc9ceefc9380c3f03c))
- **transcribe:** Pin the darwin and windows-arm64 whisper helpers of helpers-v1.9.2 ([14f638a](https://github.com/AlexGalhardo/vukem-video/commit/14f638a0add99e9ffd1e2107d19e8f05a5fef44e))
- **service:** Resolve the speech of audio.duck and the beats of audio.mark_beats ([10dcc88](https://github.com/AlexGalhardo/vukem-video/commit/10dcc88410811fd6d9a6f4b167ce4c651e609e96))
- **service:** Render documents with user fonts and refuse font.add of an unknown file ([657fe52](https://github.com/AlexGalhardo/vukem-video/commit/657fe5232b39e704a7e1b59f745e7133f36789d1))
- **service:** Keep bundle fonts in the font store and validate them on import ([5114856](https://github.com/AlexGalhardo/vukem-video/commit/5114856ff5606a332257b81e61b0ed9b9cc06dea))
- **web:** Route the project bundle export and import ([8cbd474](https://github.com/AlexGalhardo/vukem-video/commit/8cbd474a7e40ec07b1e67409e6833c542ac8c362))
- **ui:** Add the window of mounted frames and the item selection for virtualization ([8473c6b](https://github.com/AlexGalhardo/vukem-video/commit/8473c6b0fca0323fc91e820e2a2789ed93189f59))
- **ui:** Count timeline item renders through the debug hooks ([49416ae](https://github.com/AlexGalhardo/vukem-video/commit/49416ae4f96abb0dc8a87b7536abeb78d0f25b85))
- **ui:** Mount only the window of items on large projects and keep the pinned ones ([5edc7f4](https://github.com/AlexGalhardo/vukem-video/commit/5edc7f452609776cab1b09f27e8e4850afd2cd09))
- **command:** Expose the BETA build ([3d1941b](https://github.com/AlexGalhardo/vukem-video/commit/3d1941bd68ce8672a5b158ec0b3d4ac22e977644))
- **ui:** Keep rebindable shortcuts in a keymap store with conflict detection ([a6aa063](https://github.com/AlexGalhardo/vukem-video/commit/a6aa063cbeb4458293a5fa5abab73548709529be))
- **ui:** Read the editing and zoom keys from the keymap store ([18159b1](https://github.com/AlexGalhardo/vukem-video/commit/18159b13855e7888562a7e07678767566c987602))
- **ui:** Add the shortcuts cheat sheet with rebinding and reset ([515f469](https://github.com/AlexGalhardo/vukem-video/commit/515f469150f436c87bb22b77a59cac49977a1771))
- **ui:** Show empty states that name the next action ([ac96887](https://github.com/AlexGalhardo/vukem-video/commit/ac96887ac80bf2f695471d2ee054f1f0afbefa9e))
- **ui:** Show first-run tips once and bring them back from the shortcuts dialog ([5dabc4e](https://github.com/AlexGalhardo/vukem-video/commit/5dabc4e9ca2dd3a100a100b4e04f779857bf81cd))
- **cli:** Add bundle:import and call the bundle service methods directly ([dea98e7](https://github.com/AlexGalhardo/vukem-video/commit/dea98e78c65482227e97860f84cd6bbacf1f80ca))

### Fixed

- **export:** Satisfy the linters in the blur and cold-frame test code ([7743135](https://github.com/AlexGalhardo/vukem-video/commit/77431350cb180ab85846741f0213b4650c9aaa1b))
- **service:** Render a frame at the exact height of the ImageSize table ([144bada](https://github.com/AlexGalhardo/vukem-video/commit/144bada8c97ab7b81f09d087b3c34c65d94bb9c8))
- **mcp:** Name the sheet range from and until in the view description ([89be7f4](https://github.com/AlexGalhardo/vukem-video/commit/89be7f4d21ac9023af85db4eb7482c045fd220ec))
- **web:** Change the role and the disabled state of a user in one statement ([2863562](https://github.com/AlexGalhardo/vukem-video/commit/28635622555d4493fe5edbde5050541afca20f9e))
- **service:** Name the missing scope in details.scope of a 403 ([6633ae1](https://github.com/AlexGalhardo/vukem-video/commit/6633ae13ef84d2243b39d9c253a18d8bcf612790))
- **ui:** Name the slider thumbs and give them a 24 px target ([71b43ca](https://github.com/AlexGalhardo/vukem-video/commit/71b43caec76754b43f0dfae20e6dc2f8d4f57220))
- **ui:** Render the left panel tabs with their tab panels ([c8fe234](https://github.com/AlexGalhardo/vukem-video/commit/c8fe234a89c96839fac344348a6f092b4f8e4f07))
- **ui:** Give the timeline grid rows for its ruler and track headers ([06c160e](https://github.com/AlexGalhardo/vukem-video/commit/06c160e37471125708ebbdce82dd3e3c5f4962a9))
- **ui:** Keep the preview region on the dark tokens in the light theme ([4963847](https://github.com/AlexGalhardo/vukem-video/commit/4963847cfb61e51e1e2dad8e4e7f180e999d5555))
- **ui:** Let pointers through the stacked error notices and show at most three ([f7fd288](https://github.com/AlexGalhardo/vukem-video/commit/f7fd288042e3076f2bff6c92d7a74532af7f71be))

### Security

- **security:** Check the scope row of every ALPHA operation of SDD-000 in T-05 ([da88490](https://github.com/AlexGalhardo/vukem-video/commit/da884909470f370307b11ea9e2e8b306f6d97930))
- **security:** Font.add cannot name a font another user brought (T-21) ([1785578](https://github.com/AlexGalhardo/vukem-video/commit/17855787680dc512813964dc1ba9000ee4ca29cc))
- **security:** Check font ownership once per batch and before the store ([8428e55](https://github.com/AlexGalhardo/vukem-video/commit/8428e551f556dbb80273d7e91f6db75c53f0b4d0))
- **security:** Allow the sample password of the user management docs in the secret scan ([d1b0f25](https://github.com/AlexGalhardo/vukem-video/commit/d1b0f2512e6f5b9c12a5e44c7567ac52c40e2ed9))

## [0.12.0] - 2026-11-15

### Added

- **ui:** Add the tile arithmetic of the clip thumbnail strip ([14b5d95](https://github.com/AlexGalhardo/vukem-video/commit/14b5d95933ca63080ccea0589ff2fba18975ba44))
- **ui:** Read the peaks file and turn it into waveform columns ([8c69126](https://github.com/AlexGalhardo/vukem-video/commit/8c69126b506125735e46705c89c2fd1e6d273518))
- **ui:** Draw strip, waveform, shape and badges on timeline items ([4c2d514](https://github.com/AlexGalhardo/vukem-video/commit/4c2d514ee931fa835a83b68680abdf5fa16cbfa2))
- **api:** Accept to as a deprecated alias of until on the sheet and plan routes ([d1c70e2](https://github.com/AlexGalhardo/vukem-video/commit/d1c70e2dc10c31fd6c33b7c406184fe02fe4e2d0))
- **web:** Export traces and metrics through drivers/otel when the endpoint is set ([a7ad9a1](https://github.com/AlexGalhardo/vukem-video/commit/a7ad9a1e6fb678dc51a6f17564a353ad495c36b2))
- **service:** Embed the pinned FFmpeg digests for doctor --verify ([6fa92df](https://github.com/AlexGalhardo/vukem-video/commit/6fa92df461391948b5b7b309d1f923fe98c6c516))
- **ci:** Add the live agent harness for the release checklist ([b78bbee](https://github.com/AlexGalhardo/vukem-video/commit/b78bbee499be4e3962a7a2834429093fb53fbba0))
- **ui:** Add the track commands to the builders, the actions and the coverage ([9745b2e](https://github.com/AlexGalhardo/vukem-video/commit/9745b2e29a92abe29ee47a798c99b913f71bd379))
- **ui:** Add the reorder drag of a track header with a ghost row ([482e04a](https://github.com/AlexGalhardo/vukem-video/commit/482e04a67246e933d1c6afce786c3cd9d097c618))
- **ui:** Edit tracks from the header and the track panel, confirm removal of a non-empty one ([1cfdd61](https://github.com/AlexGalhardo/vukem-video/commit/1cfdd619929a778b3839bc664f5da96a161c071e))
- **ui:** Add the project builders, the rename and frame rate actions and their coverage ([5059990](https://github.com/AlexGalhardo/vukem-video/commit/5059990e3af6bac1a726b79c40807a243b2bf77a))
- **ui:** Keep the default time unit in the preferences and use it in the time fields ([c259c0f](https://github.com/AlexGalhardo/vukem-video/commit/c259c0f991d07fb22e9761c1a1179e1f48f04ebd))
- **ui:** Add the language select and the reload that keeps the playhead ([a1753c8](https://github.com/AlexGalhardo/vukem-video/commit/a1753c8f28048d457392cdb4c4aeb36cf3ac03f9))
- **ui:** Add the project panel, the top bar rename and the settings page ([d26e0ef](https://github.com/AlexGalhardo/vukem-video/commit/d26e0effbcc5607c24af23ebb00c56240d7676f0))
- **ui:** Add the audio, framing, speed, colour and silence builders and their coverage ([a0649f0](https://github.com/AlexGalhardo/vukem-video/commit/a0649f0d7b4fa826a54e8e6afa068789dc58ce8b))
- **ui:** Add the audio, framing, speed, colour and silences sections of a clip ([3bb7d43](https://github.com/AlexGalhardo/vukem-video/commit/3bb7d43d2f493687b4f0c5e8ae756939439a799b))
- **ui:** Serve the settings page and link it from the navigation ([41d0074](https://github.com/AlexGalhardo/vukem-video/commit/41d00749b160424214d9c2e404e472b6d503832e))
- **site:** Check guide command mentions and the FFmpeg pin of the docs ([ecff358](https://github.com/AlexGalhardo/vukem-video/commit/ecff35882a4f05860a480f8cfcb20fad0f2c065f))
- **raster:** Validate user fonts and resolve them by hash in the font set ([51d9c73](https://github.com/AlexGalhardo/vukem-video/commit/51d9c7363315123bb5e95c8946534a3fb5ccd6b0))
- **engine:** Store validated user fonts by content hash under fonts/ ([f973a20](https://github.com/AlexGalhardo/vukem-video/commit/f973a20dc60182eea0cc0262db1b0a3ccd8baa78))
- **ui:** Add the Add shape menu and the shape section of the inspector ([511abad](https://github.com/AlexGalhardo/vukem-video/commit/511abad5f12bc2561d7a67a07edf3f54ee9987c2))
- **ui:** Add the slot and template info builders and their coverage ([265153e](https://github.com/AlexGalhardo/vukem-video/commit/265153e5c3f191338f4b4eaa7b796d75242ac89f))
- **ui:** Add the template slot section, the slots panel and the template info ([d193949](https://github.com/AlexGalhardo/vukem-video/commit/d193949c472f507eafca83a9b37a9b11f602f989))
- **ui:** Add the template entries of the projects page ([d239a75](https://github.com/AlexGalhardo/vukem-video/commit/d239a75292f3d1280c3178d1445958c4ee572214))
- **ui:** Cancel an export, list the failed checks, name the findings and reopen a running export ([b342d3c](https://github.com/AlexGalhardo/vukem-video/commit/b342d3ced9135d421f7e2882c2230b87b5a9336d))
- **ui:** Add trash and restore to the projects page ([03f4d1c](https://github.com/AlexGalhardo/vukem-video/commit/03f4d1cdac7a13500056fc9217500fdfb0ee7a1a))
- **ui:** Add the login and API keys pages of network mode and the logout ([41da819](https://github.com/AlexGalhardo/vukem-video/commit/41da8192b28da69ec75e08af53bbca9a14c1e28e))
- **service:** Export a project as a streamed .vvbundle through an export job ([8e55daa](https://github.com/AlexGalhardo/vukem-video/commit/8e55daadb2d7bc947656c74f027aa4d510242340))
- **web:** Manage users as an administrator session and keep the last administrator ([db92a24](https://github.com/AlexGalhardo/vukem-video/commit/db92a2408c25df7cc9125c8f85bbd1b1902c2d08))
- **ui:** Sign the desktop shell window in with the one-time launch address ([02d63e4](https://github.com/AlexGalhardo/vukem-video/commit/02d63e43a6a9efbf6328b50aeac63f0a3bf58665))

### Fixed

- **web:** Accept APP_ENV=test for the test transcriber, the value vukem allows ([db6f20e](https://github.com/AlexGalhardo/vukem-video/commit/db6f20e4b1ff29fc50a4daff936cd14abc1c02c0))
- **deps:** Escape obfuscated e-mail addresses in the notices table ([25a49ec](https://github.com/AlexGalhardo/vukem-video/commit/25a49ec1e0d219699a5325a1b75258dcab9d9dbb))
- **site:** Accept the example hosts of the self-hosting pages in check:built ([edbccde](https://github.com/AlexGalhardo/vukem-video/commit/edbccde6a46f742ada050bfc0a50cbae146c09b8))
- **ui:** Spell the cancelled job state as the service does ([1a54865](https://github.com/AlexGalhardo/vukem-video/commit/1a54865fd292ad8bbfa2822ca687a1b1822c8e2c))
- **ui:** Resolve the login next target as a URL and refuse control characters ([deec384](https://github.com/AlexGalhardo/vukem-video/commit/deec38407841255156af7548d33199909c288b42))
- **site:** Say the HTTP API token applies to local mode only ([6b85c68](https://github.com/AlexGalhardo/vukem-video/commit/6b85c68ce3c0fbb9119781a9a5f9d332b804a849))
- **web:** Serve the content security policy of SDD-007 8.5.5 exactly ([7dbfba3](https://github.com/AlexGalhardo/vukem-video/commit/7dbfba361cfb4ce58a4d57959fb910e743e6173a))
- **ci:** Build the windows-arm64 whisper-cli with clang-cl ([1391432](https://github.com/AlexGalhardo/vukem-video/commit/13914329503cd75d714c2859b3a8610b6700501c))
- **export:** Give each decoder process its own channels so a restart does not race the pipe closer ([7f5594c](https://github.com/AlexGalhardo/vukem-video/commit/7f5594c709d5e3d24f576ebe99b050d3259aabe4))
- **ui:** Open the desktop window on the launch address so the session cookie is set ([3bd2e7e](https://github.com/AlexGalhardo/vukem-video/commit/3bd2e7e19eacc0d58eaa41065ca4eb75dc223cfe))

### Security

- **security:** Add threatcheck for the threat table of SDD-007 ([fff0569](https://github.com/AlexGalhardo/vukem-video/commit/fff0569ced63ae1d2ce1d34e46ef33d249b055fe))
- **security:** Add the Threat coverage job ([9d904b9](https://github.com/AlexGalhardo/vukem-video/commit/9d904b9897adacd3fe4cd0e907d57952513e7030))
- **security:** Assert the release workflow pins, signs, attests and verifies (T-27) ([2f73481](https://github.com/AlexGalhardo/vukem-video/commit/2f73481425f1a00b8e4f826e7cc9da0f1d2880c4))
- **security:** Drop the no-release-yet wording from the security policy ([435fbc1](https://github.com/AlexGalhardo/vukem-video/commit/435fbc145257cf899ba173de65138dde9bb654b6))
- **security:** Name the helper download refusals T-26 and add the wrong-digest case ([c751976](https://github.com/AlexGalhardo/vukem-video/commit/c75197690801f93c789a28ab53b513e700e87e24))
- **security:** List GET /settings among the browser pages of T-05 ([315aeab](https://github.com/AlexGalhardo/vukem-video/commit/315aeab2c2268a6ef5e84d9b770183cad50101c0))
- **security:** Name the T-10, T-13 and T-28 tests as SDD-007 section 2 does ([52a1760](https://github.com/AlexGalhardo/vukem-video/commit/52a17607e6b1d7aade7fff6ab3c1c4549e9903b9))
- **security:** Allow the login labels and the self-hosting sample values in the secret scan ([5955907](https://github.com/AlexGalhardo/vukem-video/commit/595590703ca0e3b351e1043604022c982dc50792))

## [0.11.0] - 2026-11-15

### Added

- **ui:** Add the shell process and navigation policy without a window toolkit ([4c40c96](https://github.com/AlexGalhardo/vukem-video/commit/4c40c96beaa0a636e131d064d0387b5497e334c4))
- **ui:** Add the navigation guard script for the shell window ([a9a775a](https://github.com/AlexGalhardo/vukem-video/commit/a9a775add58c101114b8862d56799cecbf377b87))
- **ui:** Add the Wails v3 desktop shell as a module of its own ([44481fb](https://github.com/AlexGalhardo/vukem-video/commit/44481fb170a5019c9ee0d3885548977817128891))

### Fixed

- **engine:** Refuse to start FFmpeg as root unless explicitly allowed ([cf87b11](https://github.com/AlexGalhardo/vukem-video/commit/cf87b1182b32cbbffbcec386a119727a839559f1))

### Security

- **security:** Allow the userinfo tricks of the desktop navigation test in the secret scan ([1feaefc](https://github.com/AlexGalhardo/vukem-video/commit/1feaefc550281867aaf90bb3b81412de285a18d6))

## [0.10.0] - 2026-11-15

### Added

- **ui:** Add the auto caption builders, the transcribe job start and the coverage ([056ae7d](https://github.com/AlexGalhardo/vukem-video/commit/056ae7da4560e403067721cbbf71ea35cabdd08a))
- **ui:** Add the auto captions panel and the local transcription install dialog ([1d0a62b](https://github.com/AlexGalhardo/vukem-video/commit/1d0a62b4c12e8a4475be75c3e2733f95a7c1e801))
- **ui:** Offer re-sync on the out-of-sync caption lint finding ([6372fdc](https://github.com/AlexGalhardo/vukem-video/commit/6372fdc831622cb490ebf5e7e063e5706fae537b))
- **ui:** Add the keyframe builders, the keyframe selection and Delete on a keyframe ([74dc2b1](https://github.com/AlexGalhardo/vukem-video/commit/74dc2b12d7fbd52f5a71d9ad2ce649a9569fa2da))
- **ui:** Add the Keyframes inspector section and the keyframe diamonds on the timeline ([93c946e](https://github.com/AlexGalhardo/vukem-video/commit/93c946ea1396b9948798504f661b2b09e7af1bd4))
- **ui:** Add the transition, preset and crossfade builders, actions and coverage ([f4292c5](https://github.com/AlexGalhardo/vukem-video/commit/f4292c5bb72f4c46a5f0a6295d1f1fbd8766dcd7))
- **ui:** Add the Transitions and Animation sections, transition marks and the Crossfade entry ([4b3985d](https://github.com/AlexGalhardo/vukem-video/commit/4b3985d9a28f77e675f48334a74068f24b8e6b75))
- **service:** Add tool and model to E_TRANSCRIBE_UNAVAILABLE details ([eda6f54](https://github.com/AlexGalhardo/vukem-video/commit/eda6f54fab2d2706fe8b338187ea36e49d068805))
- **service:** Wire the transcription API provider and a guarded test stub ([fff4bcd](https://github.com/AlexGalhardo/vukem-video/commit/fff4bcd2c26a9322b86f94a423dfd3253150cf17))

### Fixed

- **ui:** Send the nearest weight with a family that lacks the current one ([abff075](https://github.com/AlexGalhardo/vukem-video/commit/abff0759d180d403ca681553c6414e9883707761))
- **ui:** Show the plan value of animated x, y and rotation instead of a keyframe lookup ([df3ac4f](https://github.com/AlexGalhardo/vukem-video/commit/df3ac4fdecf7ce5ae20ca44035239de8dedbd424))
- **ui:** Take the install tool and model from the error details ([58157e3](https://github.com/AlexGalhardo/vukem-video/commit/58157e328a00023dacc74b8b8c23e865b77815d7))

## [0.9.0] - 2026-11-15

### Added

- **ui:** Add tools and key routes to the api client ([c464f01](https://github.com/AlexGalhardo/vukem-video/commit/c464f01f21396343eb7ad80f6a01b040de922be0))
- **ui:** Add the command palette with the batch tray and a JSON form slot ([6efc640](https://github.com/AlexGalhardo/vukem-video/commit/6efc64031c6fc22a07ac76e59e58b0e0c574b811))
- **ui:** Generate the palette form from args_schema with a time field ([40f8f9e](https://github.com/AlexGalhardo/vukem-video/commit/40f8f9e3af7c3e6f1b152d936960f58ead5dd4e1))
- **ui:** Add the inspector shell with name and timing fields and the commit rules ([b2dfd06](https://github.com/AlexGalhardo/vukem-video/commit/b2dfd06d8b63e5ccc1ef287b75dfbe91bf940fbb))
- **ui:** Add the history panel, the per-actor undo state and the panel toggles ([270c37f](https://github.com/AlexGalhardo/vukem-video/commit/270c37f0585bc0e9d36413cbbee0484080209b7b))
- **cli:** Add token:rotate and list the commands the binary dispatches itself ([ff8155d](https://github.com/AlexGalhardo/vukem-video/commit/ff8155de0e67a5cce3dbccf0fb5f27dacbb12e7b))
- **command:** Mark time args and type every command argument schema ([725ac9a](https://github.com/AlexGalhardo/vukem-video/commit/725ac9a254cdd3c4d2b253366e1be966413a3e55))
- **ui:** Run rule I6 with a failing and a passing fixture for every i18n rule ([16c7105](https://github.com/AlexGalhardo/vukem-video/commit/16c71051602eca2e30a25e12dbf0c97ec18f9fc6))
- **ui:** Snap points from the core, the snapping layer, the snap line and the N toggle ([d1edef9](https://github.com/AlexGalhardo/vukem-video/commit/d1edef9d619d3580392b4db9d6b63033b91e89d7))
- **ui:** Add the selection model, the marquee and the keyboard grid with nudge ([de84ab0](https://github.com/AlexGalhardo/vukem-video/commit/de84ab021ba486e179b9993dc17e89083a5aeb00))
- **ui:** Add the move gesture, the ghost layer and the overlap snap-back ([7df7be8](https://github.com/AlexGalhardo/vukem-video/commit/7df7be8c267d42dc4e6e3d2034648b39a578c914))
- **ui:** Add trim handles with the core limits and the out-of-source notice ([8fc400c](https://github.com/AlexGalhardo/vukem-video/commit/8fc400cd2a74164edfa98758459d34444ddf85e6))
- **ui:** Add the conflict notice, apply again and the drag cancel on a remote edit ([53bbde1](https://github.com/AlexGalhardo/vukem-video/commit/53bbde1d42cd849f4842f870983cb0190ee1b0e8))
- **ui:** Complete and review the pt-BR catalogue ([8bab534](https://github.com/AlexGalhardo/vukem-video/commit/8bab5340f6a6cc5541b58aa41f6c3d97876c1483))
- **ui:** Add a Remove toggle for nullable fields and read time from x-vv-kind ([b00aebc](https://github.com/AlexGalhardo/vukem-video/commit/b00aebcb5dda848dee6b67f6272fffd3d4b71a3c))
- **ui:** Add delete, ripple delete, duplicate, close gap and rename with the item menu ([cac5da2](https://github.com/AlexGalhardo/vukem-video/commit/cac5da21be657a54657733c5b6c1118fd2c6d20a))
- **ui:** Add the library drag, audio placement, card menu, trash group and in-use dialog ([dbb8064](https://github.com/AlexGalhardo/vukem-video/commit/dbb8064ea4cbf7307b379a1dc5251a232deff926))
- **ui:** Add the canvas hit test, the selection box and the safe-area guide ([c2528e9](https://github.com/AlexGalhardo/vukem-video/commit/c2528e9e26b0e5cfc5cf8270e87b45e32e4d48f3))
- **service:** Add description and per-actor undo flags to history entries ([cec33dd](https://github.com/AlexGalhardo/vukem-video/commit/cec33dd97e10c5384e370d7773a9d8caf14d3407))
- **service:** Read stored peaks, loudness and transcripts in the full lint ([f457504](https://github.com/AlexGalhardo/vukem-video/commit/f457504a6180387b43f423cb8503a2b29662d902))
- **service:** Give the lint the rasterizer measurer in the owner ([22d3482](https://github.com/AlexGalhardo/vukem-video/commit/22d3482048dd660364200031fa1c370dd5eabd8d))
- **service:** Log job start, end and failure through a slog logger option ([8bb4a17](https://github.com/AlexGalhardo/vukem-video/commit/8bb4a174ec1f71bc636ff36a237a93e4c6aedb18))
- **ui:** Build transform operations and the use-keyframes notice action ([68ae2f0](https://github.com/AlexGalhardo/vukem-video/commit/68ae2f0b51fb14696daa157fc106616878579199))
- **ui:** Add the inspector Transform section with mixed values and keyframe markers ([6c16208](https://github.com/AlexGalhardo/vukem-video/commit/6c16208fe3344dc9ee5649eb4a5b6f8230f513c2))
- **ui:** Add transform handles on the canvas with the outline box as preview ([b6f0e5f](https://github.com/AlexGalhardo/vukem-video/commit/b6f0e5f47fe115121660b64ec14e3354103f0f5c))
- **plan:** Report the effective scale of each layer and window series ([99edb86](https://github.com/AlexGalhardo/vukem-video/commit/99edb864599169ff8d86ae155dcd760b5a00ac84))
- **ui:** Add text items, the inline text editor and the Text inspector section ([c861429](https://github.com/AlexGalhardo/vukem-video/commit/c86142916ec4da77111b7197a2dcb36db1d044cf))
- **ui:** Edit caption words, boundaries, splits, merges, breaks and styles ([f5c21ef](https://github.com/AlexGalhardo/vukem-video/commit/f5c21ef5ae948d2222c1e6868fb8057264a6ae02))
- **service:** Show metadata tags in media_inspect inside the untrusted block ([8c9ca55](https://github.com/AlexGalhardo/vukem-video/commit/8c9ca55dd878e17a9b36e40ab512ee03d9a878f7))
- **ingest:** Keep container tags, encoder and handler_name in the probe summary ([1aa2e1b](https://github.com/AlexGalhardo/vukem-video/commit/1aa2e1b1e73bb89d0435fd07cad29de0e7ea5ce2))

### Fixed

- **service:** List installed tools under the install lock so an idle status lists every file ([f5f8132](https://github.com/AlexGalhardo/vukem-video/commit/f5f8132752b0107e198c0000bfe43b2d613c5808))
- **ui:** Let commands import ui and layout import commands in the thin-client check ([91298b4](https://github.com/AlexGalhardo/vukem-video/commit/91298b4a950968b60387d4535a86558c5faf113f))
- **ui:** Render a nullable schema type as the control of its type ([b6530d7](https://github.com/AlexGalhardo/vukem-video/commit/b6530d7a628949c2f61f757f14230eb54c51214d))
- **ui:** Keep the middle half of a narrow item free for the move drag ([0a8636e](https://github.com/AlexGalhardo/vukem-video/commit/0a8636e27a30197edc2e2fb255120cecc8985e0d))
- **ui:** Capitalize a message that starts with the actor fallback ([b601562](https://github.com/AlexGalhardo/vukem-video/commit/b601562dbbb98701e85fb7e4d8bd5246da7da15d))
- **ui:** Capitalize only the fallback actor at the start of a message ([8afc0b1](https://github.com/AlexGalhardo/vukem-video/commit/8afc0b12e3e7f117d9382104c105b7c8f06b89ed))
- **service:** Report transcript unavailable in media inspection without a provider ([4b4344b](https://github.com/AlexGalhardo/vukem-video/commit/4b4344b40ad494b38d319d0b4ff8a873a22d821b))
- **service:** Keep ingest error details and hint on assets, redacted per caller ([ff9f99b](https://github.com/AlexGalhardo/vukem-video/commit/ff9f99b4e36dca533d2dd02efba70c296bcaf1c6))
- **ui:** Require the history description and undo flags the service now sends ([8ba810c](https://github.com/AlexGalhardo/vukem-video/commit/8ba810c478ca24c0240616dbc4fab42e9e361fb6))
- **ui:** Keep the inspector heading from collapsing under a tall body ([d6a9746](https://github.com/AlexGalhardo/vukem-video/commit/d6a9746406de54ccbf12111af61dbdd276a22665))
- **lint:** Name a text item once in the safe-area message ([2ff0160](https://github.com/AlexGalhardo/vukem-video/commit/2ff016013c314bbf95b37f6bed93f6554f974d46))
- **ui:** Read the effective scale and opacity from the plan instead of the last keyframe ([31da1b1](https://github.com/AlexGalhardo/vukem-video/commit/31da1b1ba9b17be93b2adb5ee1a3ad43e85f0dd2))

### Security

- **security:** Name and widen the non-loopback runtime.json check as T-30 ([390fc3d](https://github.com/AlexGalhardo/vukem-video/commit/390fc3de82863e4ef4e379e2525b46727cd9b50c))
- **security:** Drive Host and Origin checks from hosts.txt and widen T-04 on every route ([008bb8c](https://github.com/AlexGalhardo/vukem-video/commit/008bb8cee693afc28de5748a99c8aee831b46cdc))
- **security:** Check that no secret variable reaches an error, the report or the bundle ([502257a](https://github.com/AlexGalhardo/vukem-video/commit/502257a1a891a7506d7380a33ea32bc6a92cbccf))
- **security:** Give the existing T-05, T-06, T-32, T-33 and T-34 tests their threat names ([616ed0f](https://github.com/AlexGalhardo/vukem-video/commit/616ed0f8f785e7b6b9bd4f39383f965722d90707))
- **security:** Cover idempotency mismatch (T-24) and asset access by membership (T-06) ([e176a9f](https://github.com/AlexGalhardo/vukem-video/commit/e176a9fe7e2f931f1833554e5789b70cfe012439))
- **security:** Walk the network mode scenario through the real binary ([fb429bf](https://github.com/AlexGalhardo/vukem-video/commit/fb429bfc56c25940a25d7cecd0eae13b1d6d05a4))
- **security:** Name the template archive test as T-20 ([12329ad](https://github.com/AlexGalhardo/vukem-video/commit/12329ad3c910db56e5b528cf0f717159eac4184f))
- **security:** Drive SQL payloads through every free-text input (T-23) ([2dc9131](https://github.com/AlexGalhardo/vukem-video/commit/2dc9131fa742141af0c90838075f048c8458ae4c))
- **security:** Check that errors leak nothing and give no existence oracle (T-29) ([d8e0b54](https://github.com/AlexGalhardo/vukem-video/commit/d8e0b541b7418a44b0d597918785e3616b5c01dc))
- **security:** Name the rate limit test and add the export concurrency ceiling test (T-18) ([162f16f](https://github.com/AlexGalhardo/vukem-video/commit/162f16f32a43333d0d9467608ec167b85574934c))
- **security:** Name the log line forgery test as T-25 ([019457e](https://github.com/AlexGalhardo/vukem-video/commit/019457e730815977be7471a6dac77c9b47d34161))
- **security:** Check that names reach agents only quoted or marked x-untrusted (T-08) ([7da2c75](https://github.com/AlexGalhardo/vukem-video/commit/7da2c75b73d2ff4a35cc2d7b08140fda7b56b0f7))

## [0.8.0] - 2026-11-15

### Added

- **service:** Unpack an archived helper after its digest matches and record the program digest ([5bee5dd](https://github.com/AlexGalhardo/vukem-video/commit/5bee5ddfa35a7ea59c5394269c28e43e702ae340))
- **service:** Cap idempotency keys per actor and limit event streams per key ([1a8529b](https://github.com/AlexGalhardo/vukem-video/commit/1a8529be26272b784bd7800283fe104b0f74152d))
- **service:** Add SnapPoints, FontList, ExportFile and AuditList with their routes ([ba19f23](https://github.com/AlexGalhardo/vukem-video/commit/ba19f23ff09abd2b014d1db6bfa0ff896377d2a5))
- **service:** Add Doctor with the report, verify, sweep and the diagnostic bundle ([0e2ded6](https://github.com/AlexGalhardo/vukem-video/commit/0e2ded693ff6e1480ff811758e9dc5110cf32cd4))
- **ui:** Add snap points, fonts, system and audit reads to the api client ([dd4fa14](https://github.com/AlexGalhardo/vukem-video/commit/dd4fa14a7a9494babe7012814a387687d89078c8))
- **service:** Validate the boot configuration of both modes ([81b8bbb](https://github.com/AlexGalhardo/vukem-video/commit/81b8bbbc189a953960bfe8c1c0a65b1e12816db7))
- **web:** Add the network-mode Host, Origin and scheme guard ([212c5c8](https://github.com/AlexGalhardo/vukem-video/commit/212c5c84f8e5b09c04e8067dad0457873e951ea4))
- **web:** Boot network mode with its mode, address and limit rules ([8c4a6bb](https://github.com/AlexGalhardo/vukem-video/commit/8c4a6bb69736b50aa961cf45b6b0e032475125c1))
- **workspace:** Refuse a plain-HTTP VUKEM_VIDEO_URL beyond localhost ([e3557cd](https://github.com/AlexGalhardo/vukem-video/commit/e3557cd81494f432d8b2fbfb9c2e84734b0dcc16))
- **cli:** Add transcribe, doctor, fonts, audit and tools:install ([3237031](https://github.com/AlexGalhardo/vukem-video/commit/3237031ffa707265946e9124debbbb68a148899c))
- **release:** Refuse a patch version when a commit in the range is breaking ([bbde553](https://github.com/AlexGalhardo/vukem-video/commit/bbde553f13fc480f6ffd59cc6896515619771cfb))
- **service:** Report the child limits and the sandbox in doctor ([bddc108](https://github.com/AlexGalhardo/vukem-video/commit/bddc1083b50017ed127d38aca6407eba10792643))
- **service:** Run FFmpeg under the child limits and the bwrap sandbox from the owner ([a457671](https://github.com/AlexGalhardo/vukem-video/commit/a457671b32e26e552e98e1c0164abb875052c622))
- **web:** Add users, login, sessions and the first administrator for network mode ([0d4199f](https://github.com/AlexGalhardo/vukem-video/commit/0d4199fed1dfc60acae16e135926feb856a3857e))
- **web:** Add API keys with create, list and revoke routes for network mode ([c90601a](https://github.com/AlexGalhardo/vukem-video/commit/c90601a2fff173a6916099dc1591ef7c662ca88c))
- **web:** Add rate limits by address, credential, session and heavy operation ([2541bd3](https://github.com/AlexGalhardo/vukem-video/commit/2541bd3a89a962a0ce8ef615d1064815935ec8a9))
- **service:** Read the heavy-operation limit from VUKEM_VIDEO_RATE_LIMIT_HEAVY_PER_MINUTE ([e1c5ed3](https://github.com/AlexGalhardo/vukem-video/commit/e1c5ed3edeba41c343265302696f89a59914b487))
- **api:** Mark untrusted text paths with x-untrusted in the OpenAPI document ([bd9da41](https://github.com/AlexGalhardo/vukem-video/commit/bd9da41ef676eafe92f12690db27d95741c3983c))
- **service:** Add the three starter templates, built from commands and embedded ([e9481cc](https://github.com/AlexGalhardo/vukem-video/commit/e9481ccf9c46395ff8faf822463859331e7dbc2f))
- **service:** Seed the starter templates at start-up and share them in network mode ([ae66f26](https://github.com/AlexGalhardo/vukem-video/commit/ae66f26e8a45a2b702b4bd8ef5ae806d2456a8a9))
- **service:** Add structured logging with a rotating file and sanitized text ([446c51c](https://github.com/AlexGalhardo/vukem-video/commit/446c51c9c5f3ec84d4cf39897301104e24121bc2))
- **web:** Log to the workspace file and add the actor and project to request lines ([742d80f](https://github.com/AlexGalhardo/vukem-video/commit/742d80fba2e1aa72b0a989849606cf97294fc86b))
- **web:** Build the OpenTelemetry exporter only when the endpoint is set ([536875a](https://github.com/AlexGalhardo/vukem-video/commit/536875aff2d95c9cae0931c52b4050db795307a8))
- **service:** Migrate stored projects and templates at start-up with backups and a log row ([ec44252](https://github.com/AlexGalhardo/vukem-video/commit/ec442528c31395f9539a6c09bd78e67e0202d337))

### Fixed

- **service:** Answer an aborted request as 499 instead of a server failure ([684f7ca](https://github.com/AlexGalhardo/vukem-video/commit/684f7ca1d35519a4f17aeeb66aa6e54907a8dea8))
- **service:** Do not report an import ready before its asset is in the document ([d5bd30d](https://github.com/AlexGalhardo/vukem-video/commit/d5bd30d915ad2c5bf1dd0e206279b12e7d0d4fab))
- **ui:** Treat a 499 answer as a silent abort ([0cfbc03](https://github.com/AlexGalhardo/vukem-video/commit/0cfbc03957091bfe277939e00096e6744870fa36))
- **service:** Decide doctor privilege by caller, refuse verify and bundle, run one at a time ([02023f2](https://github.com/AlexGalhardo/vukem-video/commit/02023f23166ce1276d157fc57d1f149a20bc7b6b))
- **ui:** Keep the reopen back-off when a refused event stream passes the probe ([de226dc](https://github.com/AlexGalhardo/vukem-video/commit/de226dc4d3744fe5a20d6b3a52bc8ed4ecbbe865))
- **service:** Hide the variable name of a limit from non-admin callers in network mode ([1a1b92c](https://github.com/AlexGalhardo/vukem-video/commit/1a1b92c7272267c4c48fc5173ef8e9c52e46ffde))
- **service:** Keep the address space limit unless the detected encoder is hardware ([e0faf5f](https://github.com/AlexGalhardo/vukem-video/commit/e0faf5f7eaea02bdd59ecc50e0e24dd72ac9cd3e))
- **image:** Run the image in network mode on localhost and ask for APP_KEY ([13a8f2f](https://github.com/AlexGalhardo/vukem-video/commit/13a8f2ff6a2697a116f203e398fd491f41da46e1))
- **service:** Answer 405 for E_METHOD_NOT_ALLOWED and test every row of the status table ([c0c8042](https://github.com/AlexGalhardo/vukem-video/commit/c0c8042617360278e661f9ec94fd43180ad14196))

### Security

- **security:** Check the key limits and insert the key in one serialized step ([6513d88](https://github.com/AlexGalhardo/vukem-video/commit/6513d88526902bd6680688df2ccb90d9a6240132))
- **security:** Keep configuration variable names out of job and asset events ([590148c](https://github.com/AlexGalhardo/vukem-video/commit/590148cefcfaa93fb982c62286f0a3d187af6e07))
- **security:** Refuse only failed attempts from an address that spent its failures ([27a40ea](https://github.com/AlexGalhardo/vukem-video/commit/27a40eabb7bd4ec3d9580563f5282ca68ccdee6c))
- **security:** Refuse a blocked address before verifying a key and cap frames in flight ([3ecc10a](https://github.com/AlexGalhardo/vukem-video/commit/3ecc10a8253bde5a1f87d222575046b3dc11e04b))
- **security:** Allow the login variable name and the test login bodies in the secret scan ([2381055](https://github.com/AlexGalhardo/vukem-video/commit/23810557754efa3493d6d4d7fb8e2fb62052c06d))

## [0.7.0] - 2026-11-15

### Added

- **service:** Add the transcribe job with the provider gate, the artifact and the media scope ([d4c142b](https://github.com/AlexGalhardo/vukem-video/commit/d4c142bf193ab09bd330081f550f7390d4638ebc))
- **service:** Limit the transcriptions of one actor like the exports of one key ([eb7f480](https://github.com/AlexGalhardo/vukem-video/commit/eb7f48085949853d42e6dfc261c581de0ce34fce))
- **service:** Add the human-only helper install with a pinned manifest and SHA-256 check ([6d5d1b2](https://github.com/AlexGalhardo/vukem-video/commit/6d5d1b263a2d439f5833bbfb374fc63c322f75c6))
- **transcribe:** Add the helper manifest and the explicit, verified install ([3a60d75](https://github.com/AlexGalhardo/vukem-video/commit/3a60d7595ff0e4b6b5ffdeb9303528ec50700010))
- **transcribe:** Add the optional OpenAI-compatible provider through internal/outbound ([ee792b3](https://github.com/AlexGalhardo/vukem-video/commit/ee792b3632e0b1fc7db4285d465be8901d4662c6))
- **service:** Add the untrusted text package with cleaning, blocks, budgets and provenance ([015f0a7](https://github.com/AlexGalhardo/vukem-video/commit/015f0a7b412cba2192c39272d9c70b143df7fc85))
- **service:** Add the paged and delimited transcript section of media_inspect ([f01388f](https://github.com/AlexGalhardo/vukem-video/commit/f01388f07329d53314bded542670db58a08a2188))

### Fixed

- **service:** Forbid a limited key on other jobs, set op_index on a scope refusal, check first ([27ba82a](https://github.com/AlexGalhardo/vukem-video/commit/27ba82ab73f3a569cb73e6ee3cebc668a4bc3835))
- **service:** Hide another user's job from a project-limited key behind E_NOT_FOUND ([98dc359](https://github.com/AlexGalhardo/vukem-video/commit/98dc359c9299b9199427aee26210484e5d9a7910))
- **service:** Refuse a project-limited key for the helper install and its status ([668c177](https://github.com/AlexGalhardo/vukem-video/commit/668c177189a98e3232b8a284e481de57a1798daa))

## [0.6.0] - 2026-11-15

### Added

- **ui:** Add the ALPHA reads and the media state calls to the API client ([d4bcfb1](https://github.com/AlexGalhardo/vukem-video/commit/d4bcfb1171b1ef1acdcdd7772d3441e43b4b7988))
- **ui:** Add the proposed rule I7 against untranslated pt-BR keys ([67a4bf0](https://github.com/AlexGalhardo/vukem-video/commit/67a4bf0a81891567225c977f68f0c4ed26da4103))
- **service:** Add JobCancel with its route and the media scope for own ingest jobs ([bf6502c](https://github.com/AlexGalhardo/vukem-video/commit/bf6502c445d668ff1e074cc1c222eb61329ae144))
- **mcp:** Add the job_cancel tool over JobCancel ([426d8f6](https://github.com/AlexGalhardo/vukem-video/commit/426d8f6d51599a4da61518ff54aae341e62456a3))
- **cli:** Add the job:cancel command and close its parity row ([f161cb1](https://github.com/AlexGalhardo/vukem-video/commit/f161cb1c4d6408e6c4cdc0c63ae2912c4b4c4023))
- **service:** Add TemplateSave, TemplateList and TemplateInstantiate with their routes ([6b88fb0](https://github.com/AlexGalhardo/vukem-video/commit/6b88fb0e5dd5eb1e042a4d616138023caf167fdc))
- **ui:** Add the __vv test hooks, the e2e build and a shared event stream per project ([7c5e3bf](https://github.com/AlexGalhardo/vukem-video/commit/7c5e3bf34b79b0e16e7872363520a5824b142dab))
- **ui:** Add deep links and the read-only banner of a trashed project ([87ccb84](https://github.com/AlexGalhardo/vukem-video/commit/87ccb84158c1c4736242a44b99333d94845e4aba))
- **export:** Generate the export path through os.Root and enforce the size limit ([e10f49a](https://github.com/AlexGalhardo/vukem-video/commit/e10f49abb2032950f2bcc4441613840aa261baca))
- **ui:** Add cancelJob and the strip and peaks URL builders ([30df71b](https://github.com/AlexGalhardo/vukem-video/commit/30df71b3704dd6d31d68a98b2e5c4a12f5a6e890))
- **ui:** Add the English and Portuguese titles of every command and lint rule ([a908fa7](https://github.com/AlexGalhardo/vukem-video/commit/a908fa7f0d80bc2c7d14c8a0416cdf12e6b1f2ee))
- **service:** Export and import template files under the archive policy ([73a0526](https://github.com/AlexGalhardo/vukem-video/commit/73a0526f14e4e9ab472e6307f7098a3ad5ac4729))
- **mcp:** Add the template tool for list, save and new ([5d811d3](https://github.com/AlexGalhardo/vukem-video/commit/5d811d39f59f8f521deeae0e8d858bc83e56cf13))
- **cli:** Add template:list, template:save and template:new and close their parity rows ([498b04a](https://github.com/AlexGalhardo/vukem-video/commit/498b04a38dc4282d49bb04c2fd150f82a94f062e))
- **cli:** Add media:trash and media:restore and close their parity rows ([1c6b63c](https://github.com/AlexGalhardo/vukem-video/commit/1c6b63cc26b5e01c1212aa22321c897d7021ac8d))
- **ui:** Add the desktop feature-detection page, its server and the proxy fixture ([9a20e04](https://github.com/AlexGalhardo/vukem-video/commit/9a20e0412ec8a683c4969b684ec9d77939d5fe2c))
- **service:** Add TrashEmpty, the store lock and the orphan sweep ([45e43d7](https://github.com/AlexGalhardo/vukem-video/commit/45e43d7845136514e281d46370c0b06c6eaeea1f))
- **mcp:** Add export and import to the template tool and the template:export and import commands ([59cb634](https://github.com/AlexGalhardo/vukem-video/commit/59cb6349d6bc0485df24468fb06230c61391ad0c))
- **service:** Guard writes on a full disk and add the transcribe and export ceilings ([076cb60](https://github.com/AlexGalhardo/vukem-video/commit/076cb60c8c45925a21096d6b1523113e9b9e346f))
- **docs:** Render the skill recipes from files and run them through the CLI ([42754d1](https://github.com/AlexGalhardo/vukem-video/commit/42754d16649d076c10a50013b6d03c0bf1e6c29e))
- **transcribe:** Add the provider interface and the word timing repair ([9fd108d](https://github.com/AlexGalhardo/vukem-video/commit/9fd108d86c832e7763d82f1a135484452a4537f0))
- **transcribe:** Run whisper.cpp as a separate process and judge it by its output file ([b440a6b](https://github.com/AlexGalhardo/vukem-video/commit/b440a6b9ab279721f5d25a9d97f6f735dc4330c4))
- **service:** Resolve the silences and transcript words of an edit from the artifacts ([34b5ed0](https://github.com/AlexGalhardo/vukem-video/commit/34b5ed0c5bfb3cd16ae4eb39c11e89d72fb58bc7))

### Fixed

- **service:** Cancel a shared ingest only when every waiting project is visible to the caller ([ed41906](https://github.com/AlexGalhardo/vukem-video/commit/ed41906d41e664087def1fc36205b7a594a4a63b))
- **service:** Decide the cancel of a shared ingest in one transaction and fail closed ([977019a](https://github.com/AlexGalhardo/vukem-video/commit/977019ab1856ed3145f21d298a2c43c6c0635788))
- **ui:** Refetch the library when an asset fails so the card shows code and hint ([056ad84](https://github.com/AlexGalhardo/vukem-video/commit/056ad845d3c9ff200a4e18998033d3d56a8a4ded))
- **ui:** Give the style of the Radix scroll lock the CSP nonce ([4d915e6](https://github.com/AlexGalhardo/vukem-video/commit/4d915e6800dcbf5dfec55e78ca606677ddfecaa2))
- **service:** Make templates belong to their owner ([d089e45](https://github.com/AlexGalhardo/vukem-video/commit/d089e45e9f7fbb65186b8091a695ac9ce2d7284c))
- **core:** Tell a template draft from an instance with template.instance ([e67a259](https://github.com/AlexGalhardo/vukem-video/commit/e67a259ed4ef97efc42c07757af6a88c92c33ffa))
- **service:** Mark an instantiated project as a template instance ([ad18df5](https://github.com/AlexGalhardo/vukem-video/commit/ad18df5d915d90da47f16d57f323624a6b8046b9))
- **command:** Write the diff in the form of a fetched document ([06e77a6](https://github.com/AlexGalhardo/vukem-video/commit/06e77a6c78bec03ad608604794dacb947fd09791))
- **mcp:** Name the sheet end of view until and test that no tool takes a destination ([9e60428](https://github.com/AlexGalhardo/vukem-video/commit/9e60428fc305a079173181dfcf15181e7cd103e4))
- **service:** Rename the end of a view range from to to until ([22b4839](https://github.com/AlexGalhardo/vukem-video/commit/22b4839f25cd39bb39c9865b40f07fc81c914f9c))
- **ui:** Refuse cross-origin writes to the results route of the detection server ([e1c20c2](https://github.com/AlexGalhardo/vukem-video/commit/e1c20c2f82a452023e4af561323246493b911e2b))
- **service:** Keep TrashEmpty of a project-limited key inside its project ([e1609fa](https://github.com/AlexGalhardo/vukem-video/commit/e1609fa01f9af0f7358e6ac22f3fd06cb3e0589f))
- **service:** Add the hints of conflict, limit and queue-full errors and keep sizes apart ([f280a69](https://github.com/AlexGalhardo/vukem-video/commit/f280a69e3bef6c96c7769f9f9efd0cd1d12bbf16))
- **export:** Let the decoder of a single frame end by itself before it is killed ([1f2fa7b](https://github.com/AlexGalhardo/vukem-video/commit/1f2fa7bbe8d08caef807c0b60794099ad426eb51))
- **service:** Read template archives with archive/zip, not the vukem package ([e949c9e](https://github.com/AlexGalhardo/vukem-video/commit/e949c9e0021a9b73b2f79976d0ad43ed4520bccd))
- **service:** Stop the encoder at the export size ceiling instead of after the write ([a79fddf](https://github.com/AlexGalhardo/vukem-video/commit/a79fddf9f624d2b1576e0d34b4941bb71de9c0e2))
- **ui:** Write detection results through os.Root and skip media scenarios without zscale ([88b8b54](https://github.com/AlexGalhardo/vukem-video/commit/88b8b54c4dbf7488508827c5debfc1a6d1e09c6b))

## [0.5.0] - 2026-11-15

### Added

- **command:** Add clip.remove_silences ([f696d5f](https://github.com/AlexGalhardo/vukem-video/commit/f696d5fdc571b1b89267242457c0e914e276c8e5))
- **command:** Add text.apply_preset, shape.add and shape.set ([5b15f5d](https://github.com/AlexGalhardo/vukem-video/commit/5b15f5df3534969024e55edd84130a122ad1a691))
- **command:** Add caption.set_words, edit_word, set_breaks and set_style ([282e6ea](https://github.com/AlexGalhardo/vukem-video/commit/282e6ea92f07111c6e672c35b55614c9aa9e871b))
- **command:** Add caption.split and caption.merge ([67b3289](https://github.com/AlexGalhardo/vukem-video/commit/67b32894413d1a45a02ea39e8f6645920c7b65e2))
- **command:** Add caption.from_transcript and caption.retime_from_clip ([1bea23d](https://github.com/AlexGalhardo/vukem-video/commit/1bea23dc43a490d310613838e28411f4425314f5))
- **command:** Add keyframe.set, keyframe.remove and keyframe.clear ([d2188b0](https://github.com/AlexGalhardo/vukem-video/commit/d2188b0bb3371368d2cb56c3f9a2386846a3e0c2))
- **command:** Add slot.mark, slot.unmark, slot.fill and template.set_info ([d9d921a](https://github.com/AlexGalhardo/vukem-video/commit/d9d921a56b49f1a32315528f9e00d7d8aa591e79))
- **lint:** Add the ALPHA rules, the layout measurer and the artifact reader ([c0e5861](https://github.com/AlexGalhardo/vukem-video/commit/c0e5861590f423732bc743a39361cd9cd8225bc9))
- **schema:** Write the lint rule snapshot for the surfaces ([5c4e36d](https://github.com/AlexGalhardo/vukem-video/commit/5c4e36da17e9065f85efc8cfe20fa756e9eacecc))
- **web:** Add api_keys table and audit append-only triggers ([f7d96e5](https://github.com/AlexGalhardo/vukem-video/commit/f7d96e53007577317ad67e91ed52240c5a3b6ddd))
- **service:** Trash and restore of media, pending reconcile on project restore ([5ce5afe](https://github.com/AlexGalhardo/vukem-video/commit/5ce5afe8a79ca5283780130944dca0f21247d1f0))
- **mcp:** Add the project_trash tool over ProjectTrash and ProjectRestore ([742d63a](https://github.com/AlexGalhardo/vukem-video/commit/742d63af8bb254fc0249ba632a8f8e8b107521bd))
- **timeline:** Add BETA filter, blur, markers and animated image fields ([52c57c1](https://github.com/AlexGalhardo/vukem-video/commit/52c57c1f7bbb221d17854bbbfe76be4a54a2d832))
- **timeline:** Validate user fonts against bundled families and duplicate weights ([4f8955e](https://github.com/AlexGalhardo/vukem-video/commit/4f8955ebd09ef78264eab848ac10702a7e5f5380))
- **command:** Add clip.set_filter and clip.set_blur with the plan fields ([f720279](https://github.com/AlexGalhardo/vukem-video/commit/f72027966b9733f95dec32e28e6aca1947b02a65))
- **service:** Audit the media trash and restore that go through a batch ([a090a8c](https://github.com/AlexGalhardo/vukem-video/commit/a090a8c5532e97018d16cfb80c2e28566eb4b871))
- **mcp:** Make the stdio server the owner of the workspace ([5395695](https://github.com/AlexGalhardo/vukem-video/commit/5395695a4aab56ebdf90d57b211b7877c3e71df4))
- **ui:** Switch the build to ALPHA and cover every command in the coverage contract ([c114ee3](https://github.com/AlexGalhardo/vukem-video/commit/c114ee39c7b3215590a42c95c5f1379fee5ca63f))
- **core:** Add OpenTimelineIO export and import ([f7fcbf5](https://github.com/AlexGalhardo/vukem-video/commit/f7fcbf55d2b6015e8ad769daf378154054cdc83d))
- **cli:** Generate the agent skill, snippet and instructions and add agent:docs ([4f86158](https://github.com/AlexGalhardo/vukem-video/commit/4f861586e2aed617c778fc38a11508d278e89555))
- **mcp:** Read the server instructions from the embedded agent docs ([ffee37b](https://github.com/AlexGalhardo/vukem-video/commit/ffee37bfc7e90c0fe462c912aca6adabe7cf0268))

### Fixed

- **lint:** Sweep sorted spans in the transition rule instead of every frame ([3d426a3](https://github.com/AlexGalhardo/vukem-video/commit/3d426a3d03eeda4165e65747a7043fcaa9d26583))
- **ui:** Return a stable empty list from the track selectors ([9a0749d](https://github.com/AlexGalhardo/vukem-video/commit/9a0749dce617ad5181bdede3f93e5189d54f035d))
- **ui:** Size the video texture from the element and redraw when its data arrives ([867f0c8](https://github.com/AlexGalhardo/vukem-video/commit/867f0c8fb50996f93607f94cb992e60b2edf11de))
- **command:** Keep the default registry at the Alpha build ([a2f995c](https://github.com/AlexGalhardo/vukem-video/commit/a2f995c850a25531b16fb4f1373b1bfe8f4e517d))
- **web:** Keep the Accept of image routes when the guard forces JSON errors ([343db33](https://github.com/AlexGalhardo/vukem-video/commit/343db33be004d0715b4ad2f1cfadaa844d71a7b9))
- **web:** List the owner routes in route:list ([ab6bc8e](https://github.com/AlexGalhardo/vukem-video/commit/ab6bc8e65008ab757d08904a01c2a8b4b495fb8e))
- **docs:** Drop the double blank line of the generated skill ([8f6c0a9](https://github.com/AlexGalhardo/vukem-video/commit/8f6c0a931d4e7064b5d27b03ba66b8709b507ea1))

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

[Unreleased]: https://github.com/AlexGalhardo/vukem-video/compare/v0.17.0...HEAD
[0.17.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.16.0...v0.17.0
[0.16.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.15.0...v0.16.0
[0.15.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.14.0...v0.15.0
[0.14.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.13.0...v0.14.0
[0.13.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.12.0...v0.13.0
[0.12.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.11.0...v0.12.0
[0.11.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/AlexGalhardo/vukem-video/releases/tag/v0.1.0
