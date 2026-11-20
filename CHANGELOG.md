# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.41.0] - 2026-11-20

### Fixed

- **site:** Run the binary timing on a free port by default ([138ce10](https://github.com/AlexGalhardo/stuarty/commit/138ce10ab61a9584dd88fa8a50ff15731903ca4e))
- **site:** Keep the former name out of the sidebar title of the migration page ([bd24020](https://github.com/AlexGalhardo/stuarty/commit/bd24020aa1ae0021cc6162d3b37919db062512bd))
## [0.40.0] - 2026-11-20

### Added

- **image:** Rebuild a document at a revision and diff two documents ([01d7be2](https://github.com/AlexGalhardo/stuarty/commit/01d7be2fe6d040ae96c625e52523b7c2e44ea571))
- **service:** Render the diff of an image within the budget of an edit ([b6b00dd](https://github.com/AlexGalhardo/stuarty/commit/b6b00dde2cb4317df62b265d1a718d704aa2b94f))
- **service:** Revert, checkpoints and diff of image documents ([4069e8b](https://github.com/AlexGalhardo/stuarty/commit/4069e8bc2bb386c390f6d14577e3002299689cf8))
- **api:** Expose image revert, checkpoints and diff on the three surfaces ([779822c](https://github.com/AlexGalhardo/stuarty/commit/779822cefd6d9e0bed0be2cb71423c24778c5837))
- **ui:** Revert, named versions and changes in the image history panel ([da2be03](https://github.com/AlexGalhardo/stuarty/commit/da2be036198f672677d7e3d5eb92ed053126c46f))
- **core:** Add the production kind and the briefing questions of a commercial and a long video ([c9e9255](https://github.com/AlexGalhardo/stuarty/commit/c9e92558c43c3244976f1dc07ea0aedcf25a0963))
- **web:** Add the templates of the commercial and long video recipes with golden exports ([162d27b](https://github.com/AlexGalhardo/stuarty/commit/162d27b1b5565b48bbc196a92a656aa1a96fabf3))
- **service:** Install the recipe templates as built-in in a new workspace ([228e264](https://github.com/AlexGalhardo/stuarty/commit/228e2645332a6998a2db80caf86b19ede024fdef))
- **web:** Add the product-commercial and commercial-copy skills ([192caa1](https://github.com/AlexGalhardo/stuarty/commit/192caa127ff9dfe2551b4f31f780438345da1001))
- **web:** Add the long-talking-video and chapter-titles skills ([a284ab9](https://github.com/AlexGalhardo/stuarty/commit/a284ab9affadbce6fd1b64a6cb152d1070a0f8fc))
- **core:** Add the post text kind with validation and html ([04099d0](https://github.com/AlexGalhardo/stuarty/commit/04099d029f6454a6f98dd69e26396b1faeeeae06))
- **service:** Add the blog files destination with templates ([51b20d1](https://github.com/AlexGalhardo/stuarty/commit/51b20d14a35940003f832f8eb96dddd322cbd7a5))
- **service:** Add the blog wordpress destination ([26ab2e5](https://github.com/AlexGalhardo/stuarty/commit/26ab2e5484174b3c7ab047ae9ecb9da780f673b7))
- **service:** Add the blog webhook with signing and address rules ([ed6cd02](https://github.com/AlexGalhardo/stuarty/commit/ed6cd0228b0c5dd5e950e580e0600ef885873e9c))
- **web:** Serve published posts on a read api with its own scope ([660b162](https://github.com/AlexGalhardo/stuarty/commit/660b1628346b58209c77202f20dcf6d069480c8a))
- **service:** Send posts through approval to blog destinations ([4e0866d](https://github.com/AlexGalhardo/stuarty/commit/4e0866d1109c6049818f18dfa231deca49997e7f))
- **web:** Add the post editor and the blog destination forms ([d596cf0](https://github.com/AlexGalhardo/stuarty/commit/d596cf02b2f0d9c7221e05a99215b7278fb8436d))
- **service:** Serve the briefing of every kind of production ([d79bf1e](https://github.com/AlexGalhardo/stuarty/commit/d79bf1ea63e67bbd4de78d7492ca4beea67ca602))
- **image:** Share the recipe image templates in network mode ([b8c7bb3](https://github.com/AlexGalhardo/stuarty/commit/b8c7bb3e829405d7f85691c11c73b669d1055dde))

### Fixed

- **service:** Find an image operation in command help when its name is unique ([b08bfee](https://github.com/AlexGalhardo/stuarty/commit/b08bfee3fd7ba837e5084ee4fa807df015974653))
- **service:** Answer a create in UTC and accept a date in the list range ([db74d66](https://github.com/AlexGalhardo/stuarty/commit/db74d6661a30496fda8e9973b146fba21c256d2b))
- **service:** Name the domain of an operation command help found elsewhere ([584380c](https://github.com/AlexGalhardo/stuarty/commit/584380c067960fa3e7ebfa91e11594222e25b1fa))
- **core:** Keep the millisecond of a scheduled instant ([20b3938](https://github.com/AlexGalhardo/stuarty/commit/20b393834e7a8d5794866f376d4194abd04b4a9e))
- **remote:** Send the zone of a publication list to the server ([1253d1e](https://github.com/AlexGalhardo/stuarty/commit/1253d1e3c6d3a3a3ad2ae66caed14b9e60b18194))
- **renderer:** Make the source paths of the module the same on Windows and Linux ([000394e](https://github.com/AlexGalhardo/stuarty/commit/000394e3a89faea568c636bce901156fa35282ca))
- **renderer:** Give rustc forward slashes instead of rewriting the module ([b257ca3](https://github.com/AlexGalhardo/stuarty/commit/b257ca389e9c6efe8fb0781169bc3fa46e586c95))
- **ingest:** Read a lut with the parser the export reads ([4c32367](https://github.com/AlexGalhardo/stuarty/commit/4c32367a9937b0d299111afb595de15c2b6851ff))
- **lint:** Do not call an item hidden under a blended clip ([546c7ec](https://github.com/AlexGalhardo/stuarty/commit/546c7ec91ac91926d5c3eb5c24f22832d1f97569))
- **command:** Say the stacking order in track.add and its result ([d35bda2](https://github.com/AlexGalhardo/stuarty/commit/d35bda24aefe8a30e4f565b8de07226a9e231875))
- **command:** Say one visual track in the singular in track.add stacking ([7bdf899](https://github.com/AlexGalhardo/stuarty/commit/7bdf899f349cdc70967ef424d9e0ac99d44f23ff))
- **web:** Write the recipe briefs with the headings and order of their set ([fc2abfa](https://github.com/AlexGalhardo/stuarty/commit/fc2abfa05886d4bd314dc9e96e57b0bd471efd11))
- **web:** Send a commercial with no proof to the 15 second template ([0f5ba06](https://github.com/AlexGalhardo/stuarty/commit/0f5ba0672b23137859a8f17a6870c0b6b1bb8b6c))
- **core:** Render a post in linear time and refuse a backslash in a link ([89a58ab](https://github.com/AlexGalhardo/stuarty/commit/89a58ab98d299c7f500de23d4b8b01a6a714010b))
- **ui:** Offer the posts:read scope when a key is made ([417e474](https://github.com/AlexGalhardo/stuarty/commit/417e4748746c50c71f9db4ee5ed8987109cb89a9))
- **cli:** Name the post kind wherever the text kinds are listed ([d70fd60](https://github.com/AlexGalhardo/stuarty/commit/d70fd60f59400250c6a62ca198deaab44e7297a4))
- **core:** Do not report an item below a keyed or blurred clip as occluded ([6f04228](https://github.com/AlexGalhardo/stuarty/commit/6f04228a8b13cdad020b2733f184796f618bb4a1))
- **lint:** Do not report an item below a shaken or motion blurred clip as occluded ([d8dc51e](https://github.com/AlexGalhardo/stuarty/commit/d8dc51eb7e2f846191e4410910e8d0617ae4e514))
- **ingest:** Keep the alpha in the posters of an image ([71887a1](https://github.com/AlexGalhardo/stuarty/commit/71887a1c4d594322ab504e875c9cec4d6308654f))
- **plan:** Process the sound of an item inside a composition in the preview ([4fa67c2](https://github.com/AlexGalhardo/stuarty/commit/4fa67c2323e397478757a4b1fffb37bd4d641a3b))
- **engine:** Refuse lut lines longer than the documented 512 bytes ([40755d9](https://github.com/AlexGalhardo/stuarty/commit/40755d95e8147147c1a351d94e43d0f0048eddd2))
- **web:** Show transform, transitions and audio in the composition inspector ([7c14947](https://github.com/AlexGalhardo/stuarty/commit/7c14947e42dd8b74af8019f2a19900b545b1383c))
- **cli:** Stuarty mcp takes its address from host and port like serve ([f00c92a](https://github.com/AlexGalhardo/stuarty/commit/f00c92a24c1dd108bbf608309afc724eec01daea))
- **mcp:** Name the kind of production of the briefing apart from the item kind ([5757854](https://github.com/AlexGalhardo/stuarty/commit/5757854bbaafee09a20e256cce314cde06a29bd3))

### Security

- **security:** Accept three-digit threat ids from t-100 in the threat check ([7c395a2](https://github.com/AlexGalhardo/stuarty/commit/7c395a2a133f7d8fcc6a92f380a7f9c4f39a5449))
- **security:** Refuse padded threat ids and report misnamed security tests ([11246c8](https://github.com/AlexGalhardo/stuarty/commit/11246c825001e127a478e1479c12151ce3404ece))
- **security:** Run the blog threat rows with the whole suite ([8e57f41](https://github.com/AlexGalhardo/stuarty/commit/8e57f41debf4a54809d7f01b2a546041f46f6bc4))
- **security:** Prove a shared image template is read only and shows nothing else ([9e9c279](https://github.com/AlexGalhardo/stuarty/commit/9e9c279ab79c6162c27dcc5cf16165110dddf707))
## [0.39.0] - 2026-11-20

### Added

- **mcp:** Read approvals and start publish.upload via jobs ([1342473](https://github.com/AlexGalhardo/stuarty/commit/1342473d0531984f4edf5ce1d30f18b22db72710))
- **core:** Accept a constant speed on a composition clip ([7eead8c](https://github.com/AlexGalhardo/stuarty/commit/7eead8ca01e819e2aa2351cd2d2606e28e2f1d54))
- **engine:** Follow a composition speed in its nested audio ([2922644](https://github.com/AlexGalhardo/stuarty/commit/2922644fab7df1150ee2e7aea8ff1804dd5d50e3))
- **web:** Set the speed of a composition in the inspector ([41bb3d6](https://github.com/AlexGalhardo/stuarty/commit/41bb3d64fe7f2814c0c01c68c2b45397b3557325))
- **image:** Add a lookup table to the adjustments of a layer ([2ea832b](https://github.com/AlexGalhardo/stuarty/commit/2ea832b10428b1fd0cd1fc5f1c2038c962511c7e))
- **export:** Draw the lookup table of an image layer ([bc1967b](https://github.com/AlexGalhardo/stuarty/commit/bc1967b58ad8a2a6eec5356db0a4a4fe5ab81947))
- **service:** Resolve a lookup table for an image document ([ab1a1ee](https://github.com/AlexGalhardo/stuarty/commit/ab1a1ee260939368d630823e8015ba194bec07d5))
- **web:** Choose a lookup table for an image layer ([aab7f88](https://github.com/AlexGalhardo/stuarty/commit/aab7f8803f7dbff4284e44b1dab0c8c04eaf34e6))

### Removed

- **web:** Undo the network names change to split the check from the catalogue ([8de9a06](https://github.com/AlexGalhardo/stuarty/commit/8de9a066335f0ceaec90f469c1f721a6fe7e6fa5))

### Fixed

- **ui:** Word history rows with the core description ([00eb04b](https://github.com/AlexGalhardo/stuarty/commit/00eb04b5afaef3f8e22d2d8217cf399f9f8af2c6))
- **owner:** Report a malformed master key in doctor ([93ff612](https://github.com/AlexGalhardo/stuarty/commit/93ff6124015d4d2ca94d1d04d3654a544581caec))
- **service:** Never build a scoped token near miss equal to the token ([f7abc84](https://github.com/AlexGalhardo/stuarty/commit/f7abc847c852359067b32aad4e019cf2ab9699e3))
- **cli:** Refuse user:create and user:passwd when STUARTY_URL is set ([cc96e1e](https://github.com/AlexGalhardo/stuarty/commit/cc96e1e51ecca72a99259398d4446e16f4dab8e9))
- **workspace:** Refuse STUARTY_URL when a command must own the workspace ([abba86e](https://github.com/AlexGalhardo/stuarty/commit/abba86e8cb821fab6a7bf50dc176e29837139ca1))
- **http:** Report a request the client gave up on as 499, not 500 ([e531aff](https://github.com/AlexGalhardo/stuarty/commit/e531aff25ea1cbc012e6f006ba3902b7c9234d40))
- **ui:** Load project pictures near the viewport through a queue of two ([96a0aa7](https://github.com/AlexGalhardo/stuarty/commit/96a0aa7ba0b1f81fba1b06719e2396843af56f61))
- **ui:** Withdraw a project picture that left the viewport before its turn ([c59dec2](https://github.com/AlexGalhardo/stuarty/commit/c59dec2700c3ad339acd543f1193ac1d45930f26))
- **service:** Show the speed of a composition in the concise view ([3ea88f0](https://github.com/AlexGalhardo/stuarty/commit/3ea88f0f6a6389eda399c20563b9e0d174687e97))
- **ci:** Make the no-egress yes step match the spec and download nothing ([211da5d](https://github.com/AlexGalhardo/stuarty/commit/211da5d3d182bf8c6cd5405b23ad0ee5b1ef4ea1))
- **ffmpeg:** Install by rename and reuse a matching folder instead of deleting it ([59d623f](https://github.com/AlexGalhardo/stuarty/commit/59d623f8c0b8997e57810584b6fcf22e16095d92))
- **ffmpeg:** Reuse before downloading and stop the powershell installer deleting in place ([270ad94](https://github.com/AlexGalhardo/stuarty/commit/270ad9492df9023634390e428eb78275e3abbc5d))
- **ci:** Prove in no-egress that the install route refuses the local token ([da3e1a0](https://github.com/AlexGalhardo/stuarty/commit/da3e1a0be3999f2dfe6270d4bb3b86aed19c2c79))
- **web:** Spell the network names in pt-BR as the export dialog does ([e757b8e](https://github.com/AlexGalhardo/stuarty/commit/e757b8e7b8b20a0be2b4ac8029c8bdeff089d91e))
- **web:** Spell the network names in pt-BR as the export dialog does ([28988bc](https://github.com/AlexGalhardo/stuarty/commit/28988bc66734ff04bba161e81163b2c61fe5794e))
- **repo:** Judge each first-flow harness run by what it added ([56450f3](https://github.com/AlexGalhardo/stuarty/commit/56450f328dd1a6b927cc84302b0399b745e69c2f))
- **core:** Hide internal args from schemas, name the calls in the duck hint and the subtitle text ([afc77f2](https://github.com/AlexGalhardo/stuarty/commit/afc77f244ac020c801f39c4196d0ebc12d0f4ea9))
- **service:** Say how far an asset is and which call follows it in E_ASSET_NOT_READY ([a2c472c](https://github.com/AlexGalhardo/stuarty/commit/a2c472c35b7f44d649d327a47aa9cca14cb280ff))
- **service:** Resolve the speech of an item added earlier in the same batch ([abeb21d](https://github.com/AlexGalhardo/stuarty/commit/abeb21d039004eb5d6defdf8710832d99690ca71))
- **service:** Keep the progress detail of E_ASSET_NOT_READY between 0 and 1 ([40addc4](https://github.com/AlexGalhardo/stuarty/commit/40addc4ebaf53aa5606bba21dd100b7aa56c9b90))
- **ui:** Say in the item menu and the shortcut list that mirroring is not pasted ([0759d9d](https://github.com/AlexGalhardo/stuarty/commit/0759d9d848a65d70199be0898a332523ede7df47))
- **contract:** Record the OpenAPI document without the internal resolved field ([1dcb668](https://github.com/AlexGalhardo/stuarty/commit/1dcb6686a12897cf1f54d857ac81413886188330))
- **command:** Tell a caller of caption.import_subtitles to send the content, not the path ([bc450f5](https://github.com/AlexGalhardo/stuarty/commit/bc450f5f2ab56e035412d6a5d0674bad2e431df5))
- **service:** Check the digits of a hash before they become a folder ([0fcd58e](https://github.com/AlexGalhardo/stuarty/commit/0fcd58e2238245e0dd8fcef3aea17af4bcb95f86))
- **image:** Judge the ceiling of files after the old table leaves ([a910133](https://github.com/AlexGalhardo/stuarty/commit/a910133d447f5ed1a4d5e65e3f509dbd36d3b650))
- **image:** Hold an image to the tables the renderer can hold ([e3b2b49](https://github.com/AlexGalhardo/stuarty/commit/e3b2b495195d026255c6dd715f9a286762f1f176))

### Security

- **security:** Prove mcp job tools cannot decide, update sdd-007 ([b0f7504](https://github.com/AlexGalhardo/stuarty/commit/b0f7504cb2ff4edc3e0844ccbd75d4f4de94dd97))
- **security:** Prove t-48 on mcp against a pending publication ([9f0b5ce](https://github.com/AlexGalhardo/stuarty/commit/9f0b5ce4bc406eab48b3bc164c5edb5bbcfaf1c2))
- **security:** Threats T-67 to T-69 of image lookup tables ([858d61a](https://github.com/AlexGalhardo/stuarty/commit/858d61a16b51bd9c80944f59977fe9153e44488c))
## [0.38.0] - 2026-11-20

### Added

- **service:** Pause the scheduler and count what waits ([d9310cf](https://github.com/AlexGalhardo/stuarty/commit/d9310cfda0796617f72eb5ae30fac2c28813fd64))
- **desktop:** Hide to the tray and quit with a warning ([dacd9da](https://github.com/AlexGalhardo/stuarty/commit/dacd9da8fe1b1fa2ab64b97782ffc4390aa48566))
- **desktop:** Focus the running instance on a second launch ([45d0bb0](https://github.com/AlexGalhardo/stuarty/commit/45d0bb01823c5a46131871031cbead3ef9f4b9ba))
- **desktop:** Start at sign-in with its removal ([e32a506](https://github.com/AlexGalhardo/stuarty/commit/e32a50634ea5a9342d72c1a834fc675020cc4a63))
- **web:** Ask once and set the background mode in both languages ([2fdadd5](https://github.com/AlexGalhardo/stuarty/commit/2fdadd52aff599223150728b8b1586a83cf4fe88))
- **desktop:** Remove the start at sign-in on uninstall ([e2e5b94](https://github.com/AlexGalhardo/stuarty/commit/e2e5b94f814ce778df6346137d5a29ec40fdeb58))
- **export:** Render one item before the envelope and key it ([b5c8960](https://github.com/AlexGalhardo/stuarty/commit/b5c89602c67c6216985ccbc43ab6518a2b7e82ff))
- **service:** Cache the processed audio of an item by key ([d9047d2](https://github.com/AlexGalhardo/stuarty/commit/d9047d2141c328da29d0cabb1af021527ed10492))
- **http:** Serve the processed audio of an item by key ([4218e8c](https://github.com/AlexGalhardo/stuarty/commit/4218e8c081313769d148e30b70033a6359d88718))
- **preview:** Play the processed audio of an item on the timeline ([d7bb58c](https://github.com/AlexGalhardo/stuarty/commit/d7bb58cdc362b9fe1e653294084e92ac92d4a67a))
- **preview:** Expand a plan group into render surfaces ([e0e7ac2](https://github.com/AlexGalhardo/stuarty/commit/e0e7ac296fef9ec40f2d10a8ba241650a84831da))
- **preview:** Draw group surfaces in the render worker ([7317e75](https://github.com/AlexGalhardo/stuarty/commit/7317e7511393a9ea141bdacca2059534ae0560ef))
- **preview:** Play composition groups live in the renderer ([96d0670](https://github.com/AlexGalhardo/stuarty/commit/96d06707e0017c6a72c41c541c50012f4144957d))
- **preview:** Draw composition groups in the PixiJS fallback ([8437bd1](https://github.com/AlexGalhardo/stuarty/commit/8437bd1d444239bcfb2872b9417ec6334e3f1242))
- **service:** Add the image_edit capability to the openai adapter ([b952837](https://github.com/AlexGalhardo/stuarty/commit/b952837c56f97c29070d992253e570b36764f651))
- **image:** Add layer.replace_source and the image.edit job ([878ba75](https://github.com/AlexGalhardo/stuarty/commit/878ba75802298b6ebe8be617775c443346dd32d0))
- **api:** Expose image.edit on HTTP, the CLI and job_start ([9f82beb](https://github.com/AlexGalhardo/stuarty/commit/9f82beb8c961fd7ce6b205b3c680a92144328e10))
- **ui:** Add remove background and generate to the image editor ([17653ee](https://github.com/AlexGalhardo/stuarty/commit/17653ee966571afd5d7afba3ed8d42ce007a9470))
- **core:** Add x and tiktok limits, the price of a post and sent_as_draft ([d2bb895](https://github.com/AlexGalhardo/stuarty/commit/d2bb89510b2f2fde313f5b3287c1330a08b09432))
- **service:** Add the x connector with a price and a kept refresh token ([9f9b7e6](https://github.com/AlexGalhardo/stuarty/commit/9f9b7e691a191e7b472d9bb747bc4337f1422ed6))
- **service:** Add tiktok drafts, sent_as_draft and the price of a post ([e71f28a](https://github.com/AlexGalhardo/stuarty/commit/e71f28a23a234be9952009550896d796e0d19714))
- **web:** Show the price and the draft state, with the two guides ([91a18f0](https://github.com/AlexGalhardo/stuarty/commit/91a18f0a46cb2f24eee3285e888a3bb5a0f5a02f))

### Fixed

- **desktop:** Quit on every shutdown instead of hiding ([ab96223](https://github.com/AlexGalhardo/stuarty/commit/ab96223355d0b39170fa305263a82dc186e4fdff))
- **api:** Refuse the scheduler route in network mode ([38dc447](https://github.com/AlexGalhardo/stuarty/commit/38dc447d9d52a37554355cb62ace1da0a3f1cdec))
- **preview:** Name a processed window by a frame of its file ([c75a238](https://github.com/AlexGalhardo/stuarty/commit/c75a2387851fd43797ad44a3553225d891ee10cd))
- **service:** Refuse a processed sound larger than the cache ([f70185c](https://github.com/AlexGalhardo/stuarty/commit/f70185c6e0910de4dc13d731693e633df6eb3926))
- **plan:** Put the frame rate in the processed sound key ([cb1ca65](https://github.com/AlexGalhardo/stuarty/commit/cb1ca65a5c3e870bba41591f4a11503ccbba6d05))
- **service:** Forget processed sound failures that expired ([391eea9](https://github.com/AlexGalhardo/stuarty/commit/391eea9c55aff9aee9755f1bb4d47bb24d64bfdb))
- **preview:** Cut a nested picture with the size of its source ([29bc154](https://github.com/AlexGalhardo/stuarty/commit/29bc154672923fbfc3a9e8b95bc868eb27c182b8))
- **owner:** Honour test address variables only under APP_ENV=test ([80b2abc](https://github.com/AlexGalhardo/stuarty/commit/80b2abcce69c5abba9a22f07037c2e6b7f25ba58))
- **owner:** Refuse the test addresses before the workspace ([b9fc911](https://github.com/AlexGalhardo/stuarty/commit/b9fc911342badccf99af7fb024fc3d7c3b373e93))
- **ui:** Clear social and google callback addresses early ([cccda05](https://github.com/AlexGalhardo/stuarty/commit/cccda0553dbdd00db4c714679103f3c49686514e))
- **preview:** Blend over a clear canvas like the export ([45f2d1a](https://github.com/AlexGalhardo/stuarty/commit/45f2d1acfd95565122e5fbb9575075fb8274225d))
- **preview:** Blend once over a half-covered clear canvas ([b545153](https://github.com/AlexGalhardo/stuarty/commit/b5451532853447406067724bd3dc6eca70626f09))
- **preview:** Blend inside a group over its clear surface ([6c799de](https://github.com/AlexGalhardo/stuarty/commit/6c799def8015e6a05e7d7bc6f47271085f2287f4))
- **service:** Refuse a sign-in connection for image.edit ([86f7ea0](https://github.com/AlexGalhardo/stuarty/commit/86f7ea02862b47dac77767549270f82162dc4737))
- **ui:** Settle a model job only on the image it edited ([48ddfc6](https://github.com/AlexGalhardo/stuarty/commit/48ddfc6f4d5a145ccace08453d8abfe3dec1e061))
- **service:** Refuse a layer picture the provider cannot take ([c4741c6](https://github.com/AlexGalhardo/stuarty/commit/c4741c678764e436049cb1f1b307761c794deb76))
- **ui:** Say when a model edit has no project to keep it ([00aec52](https://github.com/AlexGalhardo/stuarty/commit/00aec523bd3a2c55d168fbbe2210ef012ed3344f))
- **service:** Rotate a refresh token by a locked read and write ([5b9d327](https://github.com/AlexGalhardo/stuarty/commit/5b9d3279fb0b9acba99b00491c27aa1da3a1ec3c))
- **service:** Never post to x again after a send stopped mid-post ([d597a98](https://github.com/AlexGalhardo/stuarty/commit/d597a987bbde1fcc5c8dd817dcfb606b10ae4db8))
## [0.37.0] - 2026-11-20

### Added

- **service:** The sign-in flow, its stored form and its refresh ([3ab2522](https://github.com/AlexGalhardo/stuarty/commit/3ab25227e40dd78fd163a5c4afb8423f873a357f))
- **service:** Add the OpenRouter adapter and its sign-in ([649b380](https://github.com/AlexGalhardo/stuarty/commit/649b3800a3a64c3d153b9e76871dc23c110379a3))
- **service:** Sign in with ChatGPT for the OpenAI adapter ([339af82](https://github.com/AlexGalhardo/stuarty/commit/339af82332cc51371c8b8478a5f9caf1ac5cfee5))
- **service:** Begin, finish and list a provider sign-in ([8bfb934](https://github.com/AlexGalhardo/stuarty/commit/8bfb934ef4b222ae1f67b3d19a12d382eb400998))
- **ui:** Sign-in buttons and states on the provider cards ([d1ac9c9](https://github.com/AlexGalhardo/stuarty/commit/d1ac9c9df0eb4d444d5d061a98ca993abedcca80))
- **docker:** Add the compose stack and Caddyfile for a VPS ([39a8357](https://github.com/AlexGalhardo/stuarty/commit/39a83573dc4ecdc8e563ce4382362f75795fa814))
- **docker:** Add install.sh and its test for the VPS package ([37d1634](https://github.com/AlexGalhardo/stuarty/commit/37d16341fb82c89d331ceecf5d30d23731c0b599))
- **workspace:** Add workspace backup and restore with scripts ([9a255da](https://github.com/AlexGalhardo/stuarty/commit/9a255da206619424ab7baa9c25cfe228d4e4c080))
- **docker:** Add update.sh with a health check and rollback ([1488a6d](https://github.com/AlexGalhardo/stuarty/commit/1488a6d3a55cfb26e1cf4d8476151a6308aef293))
- **service:** Import an image file as a document or a layer ([8e13464](https://github.com/AlexGalhardo/stuarty/commit/8e134643d4728fd2ffafb0356dac017d155de219))
- **http:** Stream image events and take a file to import ([fab6c80](https://github.com/AlexGalhardo/stuarty/commit/fab6c80b050416df0de4dd344db4b1909cc48df1))
- **mcp:** Import an image from MCP, the CLI and HTTP ([679b457](https://github.com/AlexGalhardo/stuarty/commit/679b457783fc41bc758b02345de5c4426be912bc))
- **web:** Update the image editor from the event stream ([dfd3c67](https://github.com/AlexGalhardo/stuarty/commit/dfd3c67c913dc1452a0e2fa54bee507c04caf952))
- **core:** Validate and estimate a video generation ([e252799](https://github.com/AlexGalhardo/stuarty/commit/e2527995933f39783bff9925f1c9435d058a956b))
- **service:** Declare the video generator of a provider ([31c518a](https://github.com/AlexGalhardo/stuarty/commit/31c518a2136c6fba813be74804df881843546bf5))
- **service:** Run video generation as a resumable job ([71dc3de](https://github.com/AlexGalhardo/stuarty/commit/71dc3decf2065a2881972c1baa758cbc3416029c))
- **api:** Expose video generation on HTTP, the CLI and MCP ([d7e572b](https://github.com/AlexGalhardo/stuarty/commit/d7e572b252b9518e344f6b31faf1476d6c8f3e33))
- **ui:** Add the video clip tab to the Generate dialog ([03b9584](https://github.com/AlexGalhardo/stuarty/commit/03b95843f30cabc3c9185f3a090fd9e40f191660))

### Fixed

- **service:** Refuse sign in with chatgpt on a network server ([617471a](https://github.com/AlexGalhardo/stuarty/commit/617471a6fc2680dc0019d94bc55fd64a0d3e4af8))
- **service:** Keep a chatgpt sign-in to text generation ([a7042c0](https://github.com/AlexGalhardo/stuarty/commit/a7042c003967a8958ee5e40db8f174399ce8f61f))
- **ui:** Clear the sign-in address before the exchange ([94b1f1f](https://github.com/AlexGalhardo/stuarty/commit/94b1f1f39479d6d75f2bc4454c740648d6bd3678))
- **service:** Ask for a new sign-in when a token is revoked ([49b2055](https://github.com/AlexGalhardo/stuarty/commit/49b205567a7f123b47f82dff810457050af5f3f3))
- **ci:** Read verify checks from base on Git Bash, skip go.mod-only ([c7bff9d](https://github.com/AlexGalhardo/stuarty/commit/c7bff9d4e1a82c873081fae93c144ee4fd34529d))
- **ci:** Stop verify when a base check cannot be read ([bc5b923](https://github.com/AlexGalhardo/stuarty/commit/bc5b923ee08e23f052864416f82bc16e2fb4e350))
- **ci:** Build each changed go module on go.mod-only changes ([4249f2a](https://github.com/AlexGalhardo/stuarty/commit/4249f2a43c9e4997fedc1d52d4757298a0bdc316))
- **cli:** Refuse a workspace restore when STUARTY_URL is set ([aafaefd](https://github.com/AlexGalhardo/stuarty/commit/aafaefd2e6d34fe6841e07192201f15e8fe8bca6))
- **workspace:** Refuse a backup folder that holds a link ([3ba123e](https://github.com/AlexGalhardo/stuarty/commit/3ba123e8be97feae4a50eaef8d1c76236b53e31a))
- **docker:** Roll back when compose up fails on an unhealthy start ([4bad8bc](https://github.com/AlexGalhardo/stuarty/commit/4bad8bc55fd9b82a80753c741efb0abf9b381b67))
- **docker:** Never pull the image and keep backups private ([79d8bbe](https://github.com/AlexGalhardo/stuarty/commit/79d8bbe976dbdc8b57197662488839aa098406ea))
- **docker:** Check the whole value of the install input ([adfdab4](https://github.com/AlexGalhardo/stuarty/commit/adfdab427a3c6af3998aed79e71d703628715602))
- **web:** Keep the layer import open while the file is pending ([00665e8](https://github.com/AlexGalhardo/stuarty/commit/00665e88d69fa4c3711dff7802c17c5982bd3448))
- **service:** Abandon the video task of a requeued job on cancel ([e9e73c6](https://github.com/AlexGalhardo/stuarty/commit/e9e73c63d87a718bf546d377f156b7b50960c2c7))
- **service:** Poll a video task at most once a second ([0b19220](https://github.com/AlexGalhardo/stuarty/commit/0b19220152d1629edbcdfd881eecd90a538351fd))
- **ui:** Say no provider makes video instead of a dead link ([d89a50c](https://github.com/AlexGalhardo/stuarty/commit/d89a50c0a479670dca17999f87b95eef10c45ba3))
- **service:** Refuse a first frame over 20 MB before approval ([6a9096f](https://github.com/AlexGalhardo/stuarty/commit/6a9096f814a4ae24d674017849cf3af0e37064bf))
- **service:** Name the capability in the sign-in refusal hint ([0d81c8d](https://github.com/AlexGalhardo/stuarty/commit/0d81c8d7ceed50cee433c081134ad60fb72813d0))

### Security

- **security:** The sign-in threats T-75 to T-78 ([9d767b0](https://github.com/AlexGalhardo/stuarty/commit/9d767b08829c36e6ed087007bf972e031d9bb435))
- **security:** Count the sign-in threats of epic 12 ([bbb69fa](https://github.com/AlexGalhardo/stuarty/commit/bbb69fa85b412d7d301916af64cf8a0a830d5e37))
- **security:** Renumber the image upload threats to t-62..t-66 ([7975f08](https://github.com/AlexGalhardo/stuarty/commit/7975f08726df19c23908bd1951775178e1eb4132))
## [0.36.0] - 2026-11-20

### Added

- **core:** Add the pure rules of writing: kinds, scripts, captions, limits and diff ([9c10568](https://github.com/AlexGalhardo/stuarty/commit/9c10568a10121d770f33e4bdbb52f3cf0634b74b))
- **service:** Store texts with revisions and add the writing operations ([f1f3af8](https://github.com/AlexGalhardo/stuarty/commit/f1f3af84579a0854727720defc69a58ba5784fb6))
- **service:** Draft a text with a model as a new revision under approval ([da269c1](https://github.com/AlexGalhardo/stuarty/commit/da269c183dbb0cb145270801f59764cf9fa4d5f4))
- **api:** Expose the writing domain on HTTP, the CLI and MCP ([2a44eec](https://github.com/AlexGalhardo/stuarty/commit/2a44eec5b4ca34e9d30c8a974be89564cba7b0b4))
- **ui:** Add the Write section with the editor, revisions and drafts ([cab24ce](https://github.com/AlexGalhardo/stuarty/commit/cab24ce64cbf9cacf0ce36288573c3f8603ab3f1))
- **ffmpeg:** Add a libwebp still builder to the argument grammar ([884ea1f](https://github.com/AlexGalhardo/stuarty/commit/884ea1f660e956beeb5f65639e7cbf9251fb2df5))
- **export:** Encode image documents to WebP with FFmpeg ([bf51d3a](https://github.com/AlexGalhardo/stuarty/commit/bf51d3aee102a9499115c1d4794e3bbc88430826))
- **owner:** Wire the FFmpeg WebP encoder into the service ([d296954](https://github.com/AlexGalhardo/stuarty/commit/d296954acf488047aada5c7852978994b0d8ea2e))
- **ui:** Offer lossless and quality for WebP in the image export ([333b662](https://github.com/AlexGalhardo/stuarty/commit/333b6624407466542a77e85af3fb7c9064cc5459))
- **core:** Add the target states, due times and zones ([bc1de6a](https://github.com/AlexGalhardo/stuarty/commit/bc1de6ab9d69ae88633febdb1efd2b9327e2fd28))
- **service:** Add the publication tables and operations ([d14763e](https://github.com/AlexGalhardo/stuarty/commit/d14763e889ffc3769fe5b8025899a6e51f7bb88b))
- **service:** Add the publication scheduler with leases ([76d65fc](https://github.com/AlexGalhardo/stuarty/commit/76d65fc8e69557d51264036e3b49e1cdb38670eb))
- **web:** Expose publications on HTTP, CLI and the event stream ([7d4832f](https://github.com/AlexGalhardo/stuarty/commit/7d4832ff08e185214e3d2dca8732f78746504a86))
- **web:** Add the publishing calendar, list and editor ([9c09e19](https://github.com/AlexGalhardo/stuarty/commit/9c09e1907a03a389c5ab684d0dc05a91ceab288b))
- **service:** Show publications in the project overview ([e3e0957](https://github.com/AlexGalhardo/stuarty/commit/e3e0957779e98f4ded6efd6421c2ff90ce42aca6))
- **command:** Add the composition item and clip.add_composition with its nesting rules ([9c66b7c](https://github.com/AlexGalhardo/stuarty/commit/9c66b7c6cedf97aeb663107f8aa3753d25676154))
- **plan:** Resolve nested compositions into flat layers or one group layer ([3d9a35c](https://github.com/AlexGalhardo/stuarty/commit/3d9a35ce9047e218e2a82e2c5ae2916b9e6efcd3))
- **engine:** Draw the group layer on the Go compositor and the Rust renderer host ([453044a](https://github.com/AlexGalhardo/stuarty/commit/453044a037244a6ba3f427cd01447555943b7845))
- **command:** Add composition.create_from_selection and composition.expand ([65c9f47](https://github.com/AlexGalhardo/stuarty/commit/65c9f474b6f0cb01046e4775ce188eab1efc5576))
- **plan:** Mix the sound of a nested timeline as the sound of the composition ([4533d39](https://github.com/AlexGalhardo/stuarty/commit/4533d390b2e4746c8de119a21a3e218faf8a56d7))
- **service:** Read nested timelines and key caches by child revision ([077d355](https://github.com/AlexGalhardo/stuarty/commit/077d355ee8c24fce2946be2dcf797aef7e2c28d0))
- **plan:** Name the timeline that owns the asset of a nested layer ([9f3d506](https://github.com/AlexGalhardo/stuarty/commit/9f3d506659cc56895609d43de07c266449867ecd))
- **web:** Show, create, open and expand compositions in the editor ([70221be](https://github.com/AlexGalhardo/stuarty/commit/70221be450a90c9aa6216568fcfd82ee83791743))

### Fixed

- **engine:** Blend over a transparent frame as the renderer does ([35897d9](https://github.com/AlexGalhardo/stuarty/commit/35897d9fb9e08bfb8bb4fc1732aba915c907b035))
- **service:** Let a project key reach texts through a timeline of its project ([a550b38](https://github.com/AlexGalhardo/stuarty/commit/a550b384cc4f401222bee2ada5c9cdb41b6bc2c3))
- **core:** Count an emoji sequence once in the weighted length of X ([bad90ac](https://github.com/AlexGalhardo/stuarty/commit/bad90ac5ccb456fe056a86932614ce733efdf42b))
- **api:** Carry text_kind when a text item is added on every surface ([753b367](https://github.com/AlexGalhardo/stuarty/commit/753b367824096cd99473a1b96f28f05e3dc08ccf))
- **core:** Take the NFC of X from the caller so core/writing imports no x/text ([2936b45](https://github.com/AlexGalhardo/stuarty/commit/2936b45c5e5b3ae66fc5a202d030a2c2987dffee))
- **cli:** Open waits for a lock owner instead of a second serve ([447e660](https://github.com/AlexGalhardo/stuarty/commit/447e660b1c317cf6fe5f7805ea49a1aa1e90dfee))
- **export:** Refuse a WebP past the export limit before reading ([bcc65b9](https://github.com/AlexGalhardo/stuarty/commit/bcc65b9d67518091232e28a4b31c9c528515c284))
- **ffmpeg:** Report WebP option refusals under FF-15 ([8ad1352](https://github.com/AlexGalhardo/stuarty/commit/8ad135296b0b62805cd97315e0459458c42d5cf1))
- **mcp:** Accept the quality of an image export ([3e0742d](https://github.com/AlexGalhardo/stuarty/commit/3e0742d447ac757ba9b687cc8c1ec41f512ea0de))
- **service:** Report the real size of an image resized on one side ([093063b](https://github.com/AlexGalhardo/stuarty/commit/093063be3ada16d277d00e64324387bf899ccbd9))
- **service:** Expire due approvals from the scheduler tick ([46942e8](https://github.com/AlexGalhardo/stuarty/commit/46942e8469e267b7defc0af01cdbcf9a0bf9e80a))
- **web:** Rename the publication list range flag and cover the stream ([48438a9](https://github.com/AlexGalhardo/stuarty/commit/48438a9a8ca0c11a8126969e69df23aae926b900))
- **service:** Never send a queued or stale claim twice ([da944f8](https://github.com/AlexGalhardo/stuarty/commit/da944f83ae5c3a858e17631eda7a8a43c2993764))
- **service:** Say the AI flag in a scheduled post approval ([88c8403](https://github.com/AlexGalhardo/stuarty/commit/88c8403e25dc05fb18923b6445cb70568efb4761))
- **command:** Shorten the summaries of the composition commands to fit the discovery budget ([563fca9](https://github.com/AlexGalhardo/stuarty/commit/563fca93e7930671887cafa8a653304a5f2e9f27))
- **service:** Show the timeline a composition uses in the project text ([afc4352](https://github.com/AlexGalhardo/stuarty/commit/afc435228f7b63da550127e8692554937df2b74c))
- **service:** Make copies of a production use their own timelines ([809b022](https://github.com/AlexGalhardo/stuarty/commit/809b0228d34f852e02de9d7349a5155c8bbfba90))
- **service:** Hold revert and restore to the nesting rules ([d7e0a33](https://github.com/AlexGalhardo/stuarty/commit/d7e0a3321c2fd178868550a9a0879219769a1350))
- **service:** Give a spawned timeline the rows of its assets ([53c2022](https://github.com/AlexGalhardo/stuarty/commit/53c2022f5827a4c8147a89748dd0b457b0c015f1))
- **ui:** Ask for the plan again when a nested timeline changes ([bdedb19](https://github.com/AlexGalhardo/stuarty/commit/bdedb19567f284781c8b3c207306c58c9859e211))
- **command:** Restore the full summaries of the commands ([6ea845b](https://github.com/AlexGalhardo/stuarty/commit/6ea845bb6b1f2fbe265b618728f470433ac28991))
- **service:** Draw nested texts with their own user fonts ([2535895](https://github.com/AlexGalhardo/stuarty/commit/25358953a17b67af9ae8de91db1e042c1468958d))
- **ui:** Show the picture of a composition from the API ([30ce296](https://github.com/AlexGalhardo/stuarty/commit/30ce296d9cedbad52840e1da8148538a5b313af7))
- **command:** Refuse to expand a composition that is not drawn ([46c210f](https://github.com/AlexGalhardo/stuarty/commit/46c210f97e3e48bc212f04ad6e04bee84b9e79f8))

### Security

- **security:** Count the threats of epics 19 and 24 ([18ecb9c](https://github.com/AlexGalhardo/stuarty/commit/18ecb9c2ab2943d5d3a08eac65e9a251f3d2f8a4))
## [0.35.0] - 2026-11-20

### Added

- **command:** Diff two documents with classified, ranked changes ([4db17c5](https://github.com/AlexGalhardo/stuarty/commit/4db17c59787347344a1b192efefb698f1b23bbff))
- **command:** Revert a document to an earlier revision as a batch ([902a271](https://github.com/AlexGalhardo/stuarty/commit/902a2719b5be987bc29f3b35fe0bc197484492ef))
- **service:** Checkpoints on revisions and the automatic one before an agent's first edit ([dbd4c65](https://github.com/AlexGalhardo/stuarty/commit/dbd4c65e97876d06d03a1a307c5ded9ff43434b2))
- **service:** Revert a timeline to a revision and diff two revisions as text within the budget ([2c67c84](https://github.com/AlexGalhardo/stuarty/commit/2c67c84d9d39a63933aef44871780bb5ad98e8dc))
- **api:** Revert, diff and checkpoints on the CLI, HTTP and MCP surfaces ([e4d67c4](https://github.com/AlexGalhardo/stuarty/commit/e4d67c4e74da011cb53460ebbd5b74bd1440b5f6))
- **ui:** Revert to here, compare two steps and checkpoints in the history panel ([fcc9556](https://github.com/AlexGalhardo/stuarty/commit/fcc955652ec45bd80230752f3697ed6674739cfd))
- **command:** Add the audio processor chain and its five commands ([7ad4817](https://github.com/AlexGalhardo/stuarty/commit/7ad48170b94f767a2a06b324977f7c6d59f4865c))
- **engine:** Add the equaliser, dynamics, de-esser and spectral noise reduction ([3680977](https://github.com/AlexGalhardo/stuarty/commit/3680977f920e76f1e19d793bcad401f7fa4e3b21))
- **export:** Run the processor chains in the mixer and measure the audio ([3a2731a](https://github.com/AlexGalhardo/stuarty/commit/3a2731a9b914191392a1b74fe31d3c0f6c225c0c))
- **service:** Measure the audio before and after its processors on the three surfaces ([0dcafec](https://github.com/AlexGalhardo/stuarty/commit/0dcafec74f2d37f52b829fd3c335606a092e059d))
- **ui:** Build the audio processor inspector for clips, audio items and tracks ([66b2103](https://github.com/AlexGalhardo/stuarty/commit/66b2103597ff3aa844e3d24d8dac88626c694b05))
- **service:** Add the Meta sign-in, the account picker and the Graph client ([bb0f93e](https://github.com/AlexGalhardo/stuarty/commit/bb0f93e39b233ce70d20e6b57bd78b6afd7ba6b4))
- **core:** Add the Reel limits, the caption and the media facts to publish ([0b549b9](https://github.com/AlexGalhardo/stuarty/commit/0b549b943552c1669a35a633dbc7e904a278f0c4))
- **service:** Publish a Reel to Instagram in two steps at Meta ([8fb0930](https://github.com/AlexGalhardo/stuarty/commit/8fb09305a1824c236521da39e14ee7a6a516fbcf))
- **service:** Publish a Reel to a Facebook Page in three steps ([05f50ce](https://github.com/AlexGalhardo/stuarty/commit/05f50cef17c3420dfc3cc0e28159c468b4bcbdb1))
- **service:** Set the AI disclosure from generated assets and say so ([0e32f11](https://github.com/AlexGalhardo/stuarty/commit/0e32f118b316423f2c776c0a0a40d7ac97a0fb6c))
- **web:** Add the Meta setup guide, the account picker and its security tests ([24bda18](https://github.com/AlexGalhardo/stuarty/commit/24bda18f50b9f50464d076c0a6f195af38e7ad06))
- **core:** Validate and estimate image and voice generation ([b6ad228](https://github.com/AlexGalhardo/stuarty/commit/b6ad2287476f68de45b94d1b85aa7e0fc53dcf8e))
- **service:** Add image and speech generation to the OpenAI adapter ([c50a848](https://github.com/AlexGalhardo/stuarty/commit/c50a848e35040346d568a213b86c55ec32627e4d))
- **service:** Add the image and voice generation jobs with estimate, ledger and assets ([2472604](https://github.com/AlexGalhardo/stuarty/commit/247260468a887699e442ecf3d5a3e4cb0b1328c6))
- **api:** Expose image and voice generation on HTTP, the CLI and MCP ([f877fa8](https://github.com/AlexGalhardo/stuarty/commit/f877fa83d6f35dfbd199881bfc16e5f29ab47f0d))
- **ui:** Add the Generate dialog to the visual and video asset sections ([7a442a2](https://github.com/AlexGalhardo/stuarty/commit/7a442a2f36650bdce1ccc68b1cc41a4f8983e7d1))
- **auth:** Verify the Google ID token ([852bdc6](https://github.com/AlexGalhardo/stuarty/commit/852bdc6b9e8774fc6aba68871f5f23997837cbea))
- **auth:** Link a Google account from settings ([cdd059b](https://github.com/AlexGalhardo/stuarty/commit/cdd059bab3ef8ada1ae3940f1319bf3eb07d3ec3))
- **auth:** Sign in with a linked Google account ([4a57edc](https://github.com/AlexGalhardo/stuarty/commit/4a57edcec241818a03bc0ec32c04dda593d9b202))
- **ui:** Add the Google sign-in pages and the guide ([85c71a0](https://github.com/AlexGalhardo/stuarty/commit/85c71a063809cb8431750334034f1c8e712bbf96))

### Fixed

- **service:** Key checkpoints by the timelines table so a second timeline can have them ([6418299](https://github.com/AlexGalhardo/stuarty/commit/64182994d75fae8453984ae78b3bfd3518d470f9))
- **service:** Let a project-limited key reach the history of every timeline of its project ([dbe909a](https://github.com/AlexGalhardo/stuarty/commit/dbe909a8ea29d88387e454eaa4fdf46ca4e72d0b))
- **mcp:** Refuse the revert and checkpoint members on the history of an image ([26c2e3f](https://github.com/AlexGalhardo/stuarty/commit/26c2e3f5052b0052381900ef0dae3221e92e9cbf))
- **store:** Make a renamed automatic checkpoint a named one the ceiling keeps ([61c10e7](https://github.com/AlexGalhardo/stuarty/commit/61c10e75d6100410478846d5b172398322bbfe7d))
- **ui:** Hold the revert of the history panel to the revision of its preview ([5841926](https://github.com/AlexGalhardo/stuarty/commit/5841926498ed2f8127cf9395bdb8685218d1f377))
- **repo:** Fit epic 34 in the discovery budget and prove its commands on every surface ([d5a6764](https://github.com/AlexGalhardo/stuarty/commit/d5a6764977dd4dda912d7f4bd145c74d5bb55c05))
- **engine:** Measure a marked noise profile as it is decoded ([6dc42cd](https://github.com/AlexGalhardo/stuarty/commit/6dc42cd169dff38b34a0254fc4da14dd52018b6d))
- **mcp:** Refuse item_id in the image modes of view ([9c20113](https://github.com/AlexGalhardo/stuarty/commit/9c2011364fed687516ec382c9b3bc133bd985262))
- **service:** Count the sound measure as a heavy call ([5e734a1](https://github.com/AlexGalhardo/stuarty/commit/5e734a1782f8d0da23fa9706b589884d959869d6))
- **command:** Shorten six command summaries to fit the discovery budget ([455afa0](https://github.com/AlexGalhardo/stuarty/commit/455afa0980cbe8e5f773fd7ca943b6b384fe24fa))
- **service:** Take the error hints from the connector of the network ([468164f](https://github.com/AlexGalhardo/stuarty/commit/468164f1753f3b464e08667b36abe4f759d744c1))
- **service:** Send a post only to the account that was approved ([923e25a](https://github.com/AlexGalhardo/stuarty/commit/923e25af987efa683519b3e388661c33ac568c7e))
- **service:** Fetch only the images that were asked for ([c7a9480](https://github.com/AlexGalhardo/stuarty/commit/c7a948069aab1b797f3fdf64ba35251113d4c9c4))
- **mcp:** Shorten what job_start says about the generations ([e178821](https://github.com/AlexGalhardo/stuarty/commit/e17882139916101c17a1b2e9dbec1c831706ba6f))
- **auth:** Keep Google linkings apart from anonymous sign-ins ([712cfb0](https://github.com/AlexGalhardo/stuarty/commit/712cfb0106416e07f6f7d7d59fb3e33da528dc68))

### Security

- **security:** Cover the Google sign-in threat rows ([a1aa962](https://github.com/AlexGalhardo/stuarty/commit/a1aa962c6769b85fc07630bbe2f240ce64a980a5))
## [0.34.0] - 2026-11-20

### Added

- **core:** Add the pure rules of the hosted agent chat ([d8a05e8](https://github.com/AlexGalhardo/stuarty/commit/d8a05e87067355052518b282da7a9516031f3e2d))
- **service:** Add the Agent Client Protocol client and the fake agent ([cb82a10](https://github.com/AlexGalhardo/stuarty/commit/cb82a10cc97f5e6ea1ee27d5c934e395246e51df))
- **service:** Supervise the hosted agent process, its sessions and its transcript events ([b23c6d1](https://github.com/AlexGalhardo/stuarty/commit/b23c6d14bcd736cffb468d65e1ca06d21549f297))
- **service:** Issue the project-scoped session token and hand the MCP address to the agent ([af13921](https://github.com/AlexGalhardo/stuarty/commit/af13921992f582379a7eb7c6b554849212d27de7))
- **service:** Store transcripts and expose the chat operations and the SSE stream ([1bc60c7](https://github.com/AlexGalhardo/stuarty/commit/1bc60c72546aeb1b221369500ff8ca87a10afb05))
- **web:** Add the agent chat panel to the dashboard ([cd85c28](https://github.com/AlexGalhardo/stuarty/commit/cd85c28adc435cfc75669900792dfd8a68c919b1))
- **ui:** One shortcut registry with two presets, remapping and a searchable dialog ([773a769](https://github.com/AlexGalhardo/stuarty/commit/773a76932a625eb76f53c9a3dd8edaf354b7901a))
- **ui:** Add the first-run tour, skippable and replayable from the Help menu ([c1e85a1](https://github.com/AlexGalhardo/stuarty/commit/c1e85a1c0c38e461696fd0e7db717b6e8f03b06d))
- **core:** Add the pure rules of a publication ([f6ce317](https://github.com/AlexGalhardo/stuarty/commit/f6ce317e835916042f4fa8b8e9fdfae8a29a2b57))
- **service:** Add the connector interface, its registry and a fake connector ([fc19bfe](https://github.com/AlexGalhardo/stuarty/commit/fc19bfe35f9fd3f8f6dfad349cc856e5a3e97a3a))
- **service:** Sign in to a social network with the user's own app ([d33da1a](https://github.com/AlexGalhardo/stuarty/commit/d33da1a283668cbe51241b62518c10b3ffc4d37b))
- **service:** Upload to YouTube with a resumable session, a thumbnail and the status ([26306c0](https://github.com/AlexGalhardo/stuarty/commit/26306c0fa330cb2622dc1ebbb397af2c3d1cacda))
- **service:** Add the publish.upload job that waits for approval ([60c7b94](https://github.com/AlexGalhardo/stuarty/commit/60c7b94f2d5812506125bb59a46d181117d36ae1))
- **web:** Expose the social networks and publishing on HTTP and the CLI ([161752e](https://github.com/AlexGalhardo/stuarty/commit/161752e0c52acf78649d94ee4685df05e4607e16))
- **web:** Add the social setup guide and the publish dialog ([989cea7](https://github.com/AlexGalhardo/stuarty/commit/989cea727a3881bb306f8d6c2cd0e031a6ba5138))
- **service:** Give an external motion render what it must fit ([135af25](https://github.com/AlexGalhardo/stuarty/commit/135af2521ed6c0f1afcc2cfbef8bd2a476d8b569))
- **ingest:** Report a file that declares alpha and has none ([f113973](https://github.com/AlexGalhardo/stuarty/commit/f11397365d77bd0dad7d20ad31eb97ac7a33e521))
- **service:** Check a rendered file against the brief ([647e009](https://github.com/AlexGalhardo/stuarty/commit/647e0093292534817bca5d1edd2f2215bcb0d74e))
- **command:** Replace the media of a clip when an animation is rendered again ([80b0ab5](https://github.com/AlexGalhardo/stuarty/commit/80b0ab520e2d7eb80ed9fa6ae73941572d51a903))
- **library:** Add the skill for an animation rendered by an external engine ([185a759](https://github.com/AlexGalhardo/stuarty/commit/185a75997b3e4e3d4a6da637852aa6566a6e49be))

### Fixed

- **ui:** Give the select popup the nonce of the page and a layer above the dialog ([cdf7d2e](https://github.com/AlexGalhardo/stuarty/commit/cdf7d2ef721f42ff81f966c5f0b34f9e102280b1))
- **ui:** Select a tab when an arrow key focuses it ([ef896a6](https://github.com/AlexGalhardo/stuarty/commit/ef896a60e9c4c90bcb597ea21e020e8bf3a13709))
- **e2e:** Expect the ripple of the Q trim in keyboard-only-edit ([204df06](https://github.com/AlexGalhardo/stuarty/commit/204df06c209c3225b94234ee47500619027411ce))
- **e2e:** Follow the Base UI dialog and the icon-only toolbar in wcag22 ([4d27165](https://github.com/AlexGalhardo/stuarty/commit/4d27165861a02191cedab64ff5a4ba3f605c9ddc))
- **ui:** Put the label of the track chooser in a menu group ([bbf278c](https://github.com/AlexGalhardo/stuarty/commit/bbf278c8d415e0ed6492b3917881a8869f0e8213))
- **ui:** Give a command with no arguments an empty form in the palette ([f9f7d5d](https://github.com/AlexGalhardo/stuarty/commit/f9f7d5d7a141a8b6fed342594b81910c3ff59501))
- **ui:** Show skeletons, not empty sentences, while the project loads ([caac7c8](https://github.com/AlexGalhardo/stuarty/commit/caac7c8f10db859b62c6afadf14ada24cf6a3826))
- **e2e:** Follow the Base UI redesign in four editor specs ([54003b1](https://github.com/AlexGalhardo/stuarty/commit/54003b16864209282b73211d70c34a4c40097225))
- **e2e:** Search the shortcuts dialog with the modifier the platform shows ([7855b9a](https://github.com/AlexGalhardo/stuarty/commit/7855b9a70e9456b384f94921bb434f97cbac24f6))
- **e2e:** Load an empty project in loading-states ([4b4b6bb](https://github.com/AlexGalhardo/stuarty/commit/4b4b6bb13db705970f1b9346f5b816646c6ae3f1))
- **ui:** Refuse a key a player row and a timeline row would share ([5731306](https://github.com/AlexGalhardo/stuarty/commit/5731306bdccd2e3f0642b19169ba3774b474ebe3))
- **ui:** Name the shortcut button by its prompt while it waits for a key ([3979c92](https://github.com/AlexGalhardo/stuarty/commit/3979c92ef6c7438b6baa081775741fb6131c25a3))
- **ui:** Keep the contrast of a hovered primary button in the light theme ([2715dcc](https://github.com/AlexGalhardo/stuarty/commit/2715dcc04e8ab060d3736c7b4e2bd797710a4228))
- **library:** Compose asset tags in the service, not in core ([3b3051e](https://github.com/AlexGalhardo/stuarty/commit/3b3051e58144a65a82ce8515ab0f77c8f0fc790d))
- **library:** Index a file for the owner of a copy or a template fill ([bd545a5](https://github.com/AlexGalhardo/stuarty/commit/bd545a55196f969a0db5644c0d50c2713ca471ce))
- **service:** Resolve the timeline to its project before holding a limited key to it (VV-250) ([276fe5f](https://github.com/AlexGalhardo/stuarty/commit/276fe5fddd4444dbe0fb90baea636fa5c56fe988))
- **service:** Close token leaks, start races and request floods of the hosted agent ([d2d8a7a](https://github.com/AlexGalhardo/stuarty/commit/d2d8a7a97a6e3623064d251bd21bdd603c323ca4))
- **service:** Send only the approved file and key the session on the whole post ([2fe2c27](https://github.com/AlexGalhardo/stuarty/commit/2fe2c27ab216c5783f2461dfca74c22e7e98bc86))
- **service:** Sweep the upload sessions older than seven days ([5c0bc88](https://github.com/AlexGalhardo/stuarty/commit/5c0bc8846ff5440a7cac7db7fac5e0d9e3ebe49c))
- **web:** Draw the social sign-in page without the session cookie ([2dafb4a](https://github.com/AlexGalhardo/stuarty/commit/2dafb4a2df8a4d6da7e055f71e38cd5694f41dad))
- **service:** Build the upload key without an unchecked marshal ([d1a6d1d](https://github.com/AlexGalhardo/stuarty/commit/d1a6d1d0f1f530ccb338ca4880679a8308f29599))
- **web:** Name the new clip command in the catalogue and the coverage table ([7bdd848](https://github.com/AlexGalhardo/stuarty/commit/7bdd848b44edb52b24501040fa6cc4a595cf2b20))
- **ingest:** Report missing alpha only for a file that declares it ([95a41cb](https://github.com/AlexGalhardo/stuarty/commit/95a41cbb3bb83684953e2de6228784bd8cc632bc))

### Security

- **security:** Cover the chat threats T-70 to T-74 ([302e9ea](https://github.com/AlexGalhardo/stuarty/commit/302e9eac18c9cf9d0923a31cbdaa5b9b6e3c5677))
- **security:** Key the asset library side by owner and hash ([1adc215](https://github.com/AlexGalhardo/stuarty/commit/1adc215b1f093a667b6cbc3b7dc3c8b2f764229c))
- **security:** Name a reuse by hash with the caller's own name ([b377357](https://github.com/AlexGalhardo/stuarty/commit/b377357cf9c2e225baac7cb4e1c3f149fd3476fa))
- **security:** Resolve a timeline id to its project for every key check ([b0a543e](https://github.com/AlexGalhardo/stuarty/commit/b0a543e6016934a6c5a409dc1cc36661332968ed))
- **security:** Reach every timeline of a key project in library and approvals ([d34b77c](https://github.com/AlexGalhardo/stuarty/commit/d34b77cb912a84195ccaaec9a65f8fa6411a5d2b))
- **security:** Cover the publishing threats T-56 to T-59 ([a039d04](https://github.com/AlexGalhardo/stuarty/commit/a039d04a31b64f6d245a63a513d9b850a8ea9057))
- **security:** Count the publishing threats in the model ([740207f](https://github.com/AlexGalhardo/stuarty/commit/740207f614733f19f8eb508ab0798f869995b966))
- **security:** Count the hosted agent threats in the model ([868637f](https://github.com/AlexGalhardo/stuarty/commit/868637f00652d2a54881b1ca977d9b03c1db5866))
- **security:** Reach the second timeline of a limited key in the motion hand-off ([627393e](https://github.com/AlexGalhardo/stuarty/commit/627393ef03aff3390189b3ce617013c5ed060c63))
## [0.33.0] - 2026-11-20

### Added

- **core:** Add the pure rules of the AI provider layer ([ed6ffb0](https://github.com/AlexGalhardo/stuarty/commit/ed6ffb08fa99b33487eef759f0d8efed2ac2d72d))
- **service:** Add the provider registry, catalogue and price table ([b8a0241](https://github.com/AlexGalhardo/stuarty/commit/b8a0241bb4dc210b059d0510b4aa7094622f0e5c))
- **service:** Add the OpenAI and Anthropic text adapters ([7fb8dd6](https://github.com/AlexGalhardo/stuarty/commit/7fb8dd6b633a870a9d21d41164e1f8c088db274a))
- **service:** Add the text.generate job with estimate, ledger and ceiling ([0a06cdc](https://github.com/AlexGalhardo/stuarty/commit/0a06cdc69d7eda4db83d8d003c8f101e1d5234b5))
- **api:** Expose the AI providers on HTTP, the CLI and MCP ([ea0302e](https://github.com/AlexGalhardo/stuarty/commit/ea0302e6d447c78f9450799e43de67ad2b46ce2a))
- **ui:** Add the AI provider cards to the Connections page ([bea141c](https://github.com/AlexGalhardo/stuarty/commit/bea141c576607e1e84500ba843719da3fe1108ec))
- **core:** Add the pure rules of the asset library ([0aea240](https://github.com/AlexGalhardo/stuarty/commit/0aea2409d0818ed7bb14145b501520b07d3e8ca5))
- **web:** Add the asset library tables and the migration of existing workspaces ([f6c4fe9](https://github.com/AlexGalhardo/stuarty/commit/f6c4fe94dae8aa19403814a01d9b41b9fc2f4320))
- **service:** Add the asset library operations, origin on import and reuse by hash ([c12cef2](https://github.com/AlexGalhardo/stuarty/commit/c12cef21354dadbb2f93e993fefbab6e87a363fd))
- **api:** Expose the asset library on HTTP, the CLI and the media tools ([511ad3f](https://github.com/AlexGalhardo/stuarty/commit/511ad3fec19a5960f4b62f9e26cc6a89eced6706))
- **ui:** Add the Visual Assets and Video Assets sections of the asset library ([1879755](https://github.com/AlexGalhardo/stuarty/commit/18797554864dc191a84cfee307cd515877f0caed))
- **service:** Ask the briefing and store its answers as the brief ([6eaeb63](https://github.com/AlexGalhardo/stuarty/commit/6eaeb639f6ce6bd9eaf0eb1b8ecaed167a1fb945))
- **ui:** Ask the guided briefing on the overview of a project ([355b49e](https://github.com/AlexGalhardo/stuarty/commit/355b49e674ba68eedaaad03e7b5d4814d821a76e))
- **library:** Install the built-in skills and keep a person's edit ([b9e6753](https://github.com/AlexGalhardo/stuarty/commit/b9e67532653acad0e86d05dc108efdb440378877))
- **library:** Write the router, briefing, recall, retro and fetch-clip skills ([f7d7078](https://github.com/AlexGalhardo/stuarty/commit/f7d7078979bafc4319b5891aac2dc5fe65b25b96))
- **library:** Write the short vertical video, captions and thumbnail skills ([63a42ad](https://github.com/AlexGalhardo/stuarty/commit/63a42ada7b9c8955b26532b4c65f2ae61a1db162))
- **library:** Adapt the writing skills with attribution and keep the licences ([b90d41d](https://github.com/AlexGalhardo/stuarty/commit/b90d41d4b0676941a21e24adf5e2fc2126771dc0))
- **command:** Give every registry a domain and every operation one ([9fcb44a](https://github.com/AlexGalhardo/stuarty/commit/9fcb44a6d1bb50c7c20fcd6945be9182caea8e97))
- **mcp:** Route edit and history by domain and drop the operation enum ([7ab60e4](https://github.com/AlexGalhardo/stuarty/commit/7ab60e4e1794cabcc79216438a93525764e115c2))
- **mcp:** Add the discovery call by domain to command_help ([e9c1386](https://github.com/AlexGalhardo/stuarty/commit/e9c1386cb325259c6ba96d8e6f55995f1f10c082))
- **cli:** Group the commands by domain and keep the former names as aliases ([9ff15b1](https://github.com/AlexGalhardo/stuarty/commit/9ff15b174bfdb7dd618a99661959d0ada1f4b728))
- **command:** Check the agent surface budget and wire it into lint ([7a20538](https://github.com/AlexGalhardo/stuarty/commit/7a205386c9f18223196348eb30f70b66938ba252))
- **cli:** Generate one agent guide page per domain ([89d4b7f](https://github.com/AlexGalhardo/stuarty/commit/89d4b7f0ac873e787bb5af13787b78d591a32e4e))
- **image:** Add adjustments, filters, masks and blend modes to image layers ([913b380](https://github.com/AlexGalhardo/stuarty/commit/913b380149ef13f9dcd3604bb66c221524a3fa27))
- **renderer:** Draw image adjustments, filters, masks and blend modes ([45cb601](https://github.com/AlexGalhardo/stuarty/commit/45cb6017e964fd87d60632537581c7e0a27e79c4))
- **contract:** Reach the image looks through the HTTP API, the CLI and MCP ([7d13f55](https://github.com/AlexGalhardo/stuarty/commit/7d13f55f285c60777358fcca95af76bf50f83dab))
- **ui:** Add the adjustment, filter, mask and blend controls to the image inspector ([a9cb07b](https://github.com/AlexGalhardo/stuarty/commit/a9cb07bbce285ed86067ba7fc9dae5f00dedcad1))

### Fixed

- **mcp:** Fit tools/list in its budget with the asset library ([c859da1](https://github.com/AlexGalhardo/stuarty/commit/c859da1da29489c35cd82d949527995fa58bed0a))
- **service:** Stop the text of asset uses printing an empty entry line ([d54eacf](https://github.com/AlexGalhardo/stuarty/commit/d54eacfb98b2f0c6a965a0b9ed6a3cadd8238800))
- **service:** Keep a brief written by hand with the briefing headings ([4dcbc3a](https://github.com/AlexGalhardo/stuarty/commit/4dcbc3a634d64ed0ed0dcaf1adb28978549658f6))
- **library:** Correct the commands and arguments the built-in skills give ([05d3981](https://github.com/AlexGalhardo/stuarty/commit/05d3981fd634aa387785ba6852388e98932d0d39))
- **service:** Name every argument of text.generate when connection_id is missing ([d4e8fcc](https://github.com/AlexGalhardo/stuarty/commit/d4e8fccf173f0eff5d3e918d89547e64fa2c8168))
- **service:** Refuse a price table with two prices for one model ([e46a656](https://github.com/AlexGalhardo/stuarty/commit/e46a65662dc5594ff988db115b89d0860ae39261))
- **command:** Stop telling agents the publish and provider domains have no operations ([e47f77f](https://github.com/AlexGalhardo/stuarty/commit/e47f77f81488db61820d7152f3b99209c00afd7d))
- **cli:** Hold the workspace lock in the fake owner of the client-mode tests ([92c839e](https://github.com/AlexGalhardo/stuarty/commit/92c839ee783827b19fd1bb2df678dd6983df2406))
- **mcp:** Count job_start in the provider domain it starts text.generate for ([1760ec2](https://github.com/AlexGalhardo/stuarty/commit/1760ec27c41d4d66e88262030c1817b58a6abfbe))
- **image:** Keep a mask source whose opacity rounds to none as a stencil ([19a7233](https://github.com/AlexGalhardo/stuarty/commit/19a7233a2665bf1727f83c3ab0f6089f6c7bef7b))
- **image:** Name the first shape member in the error of a layer mask every time ([688a943](https://github.com/AlexGalhardo/stuarty/commit/688a9432740d1b5de3b70a6928b691df15f61c3c))
- **ui:** Plan each mask field against the document of its turn ([01af55e](https://github.com/AlexGalhardo/stuarty/commit/01af55e0fb9a2e8082c055c4e3c36763ea66c848))

### Security

- **security:** Drop a model, an answer or a code that echoes the key ([4d91537](https://github.com/AlexGalhardo/stuarty/commit/4d91537f5c2d2ce290f6bac69b26a95741144a48))
- **security:** Keep the model of an answer only when it is a model id ([d6754a7](https://github.com/AlexGalhardo/stuarty/commit/d6754a7056b74e724ed1e23367d281f9a737234e))
- **security:** Keep a saturated day of spending from wrapping past a ceiling ([bc76955](https://github.com/AlexGalhardo/stuarty/commit/bc769552ce4b7f263e3760b434c14166cd7cbedb))
## [0.32.0] - 2026-11-20

### Added

- **export:** Add the export presets and the fit check to the core ([b44dd8e](https://github.com/AlexGalhardo/stuarty/commit/b44dd8e1fc36e5b28f283e96e4951da38b7a429c))
- **ffmpeg:** Detect HEVC and AV1 encoders on an allow list ([9a9b40e](https://github.com/AlexGalhardo/stuarty/commit/9a9b40e641e5c5df513cbcce1e2bbbdd70d5fc4a))
- **export:** Write HEVC and AV1 and check the preset on the file ([0c44e9f](https://github.com/AlexGalhardo/stuarty/commit/0c44e9f676a09d3bfb923d25b65530845863d33a))
- **service:** Run export presets and batches as parent and child jobs ([3c917b9](https://github.com/AlexGalhardo/stuarty/commit/3c917b9a2a15f36cda6fff0ffda07bcecfa4ed1e))
- **web:** Expose export presets, the fit check and the batch ([32199c2](https://github.com/AlexGalhardo/stuarty/commit/32199c237e692a1e49a2bf92d625531e3b46a4a8))
- **ui:** Choose platforms, codec and batches in the export dialog ([edae700](https://github.com/AlexGalhardo/stuarty/commit/edae70059e89f837b0e62c021682023469c9701e))
- **service:** Add the pure approval decision and cost ceilings ([68726d9](https://github.com/AlexGalhardo/stuarty/commit/68726d93f56911acd5f7e6414120e87cf3bcf3df))
- **service:** Add the approval tables, operations and waiting_approval state ([82d1e98](https://github.com/AlexGalhardo/stuarty/commit/82d1e98c17123bbddcdfbee02fa9d1cc1a42a85e))
- **web:** Expose the approval queue on HTTP, CLI and MCP ([ed7d0cf](https://github.com/AlexGalhardo/stuarty/commit/ed7d0cfd2dcbc6ebcf4d32c9b066fc4ccd75d60a))
- **web:** Add the approval queue page with rules and cost ceilings ([3cf88b4](https://github.com/AlexGalhardo/stuarty/commit/3cf88b455427f8d13038657858f8246024353e11))
- **web:** Add the image frame, history and file reads and the edit-image pages ([997ff0a](https://github.com/AlexGalhardo/stuarty/commit/997ff0a34b131a9797e3785142dc2228bac82438))
- **web:** Build the image editor page, canvas, zoom, pan and transforms ([de7853e](https://github.com/AlexGalhardo/stuarty/commit/de7853e76addeb691fde8761ef22b6334215dfff))
- **web:** Build the image layer panel as a tree with keyboard and drag moves ([766b55c](https://github.com/AlexGalhardo/stuarty/commit/766b55c1e70fdd7c2fe03e0eb224bf61bf363730))
- **web:** Build the image inspector and the buttons that add layers ([07010f0](https://github.com/AlexGalhardo/stuarty/commit/07010f06d4e93b8c82f3dc98bfc35908e40e60c9))
- **web:** Build the image export dialog with findings and a download ([b6081ea](https://github.com/AlexGalhardo/stuarty/commit/b6081ea84ef2695b3714b212b5658b9419f126a4))
- **web:** Add the image history, the editor keys and the accessibility specs ([38038cb](https://github.com/AlexGalhardo/stuarty/commit/38038cb591f0ad64e34c087f7322c3684d90e554))
- **ui:** Make an image template from the Edit Image list ([25d6dc8](https://github.com/AlexGalhardo/stuarty/commit/25d6dc8fc6cf318c9df84172d4161546d3221fba))
- **timeline:** Evaluate a cubic bezier easing in fixed-point arithmetic ([4c2de97](https://github.com/AlexGalhardo/stuarty/commit/4c2de971a23a4b3353552849c41ef919f7bc2e3d))
- **command:** Set a keyframe easing with keyframe.set_easing and a bezier curve ([280b5d8](https://github.com/AlexGalhardo/stuarty/commit/280b5d83e18f6cd80fc9bbfd8bae95591bfdc82b))
- **command:** Apply layer animation presets as ordinary keyframes with animation.apply_preset ([9fcf6cf](https://github.com/AlexGalhardo/stuarty/commit/9fcf6cf6c105364401842a6df6737a60b1deed11))
- **ui:** Edit a keyframe curve, keyframe the crop edges and apply motion presets ([ae7e4a9](https://github.com/AlexGalhardo/stuarty/commit/ae7e4a9f008678e713a383f9d9d9803b25c9722b))

### Fixed

- **ffmpeg:** Never take an HEVC or AV1 encoder as the H.264 one ([7e99997](https://github.com/AlexGalhardo/stuarty/commit/7e99997d960c886b3db1e1822f249527322af9f3))
- **store:** Keep the jobs rebuild safe beside the other migrations of jobs ([1996c28](https://github.com/AlexGalhardo/stuarty/commit/1996c28c8ee58236e35e0eade5937026c3275bbd))
- **ui:** Send the codec of a plain export only with an mp4 ([2e42c88](https://github.com/AlexGalhardo/stuarty/commit/2e42c88b55b404980c181f10e442557209415c9c))
- **mcp:** Let lint check a range against an export preset ([55eb9af](https://github.com/AlexGalhardo/stuarty/commit/55eb9afb4a406de12eb90a89fe1ebe860e2a3ac9))
- **service:** Give the preset limits in the list text as the sources state them ([7f35f7e](https://github.com/AlexGalhardo/stuarty/commit/7f35f7ea17e9c7c1a77cecbadfaf48ea474f01fe))
- **cli:** Keep every --preset of export:batch ([23eb16d](https://github.com/AlexGalhardo/stuarty/commit/23eb16da7f5cd0c8d8ac07789236eb8eb42880d6))
- **store:** Leave a jobs table that has no closed list of kinds alone ([16868cc](https://github.com/AlexGalhardo/stuarty/commit/16868cc0621f526b0a1511adc7174a6ec4a4749e))
- **service:** Release the reservation of an approved job cancelled before it runs ([e85ba56](https://github.com/AlexGalhardo/stuarty/commit/e85ba56cf47fa7d7ff26bc61ac39e3db9a6c22c1))
- **service:** Record what an interrupted run spent before it was requeued ([4d87eb6](https://github.com/AlexGalhardo/stuarty/commit/4d87eb6cd96850d38ef59cbb219da1d2d8545972))
- **mcp:** Declare the waiting_approval state in the job tools output schema ([7f793df](https://github.com/AlexGalhardo/stuarty/commit/7f793dfe27415234d0d00c7b790bd1b0ab05efec))
- **service:** Refuse an unshowable approval summary as a validation error ([9609b7f](https://github.com/AlexGalhardo/stuarty/commit/9609b7f08e267f5cebbadd28d469839b27999429))
- **service:** Cancel a job that was approved while its cancel was on the way ([b4c658e](https://github.com/AlexGalhardo/stuarty/commit/b4c658ebdb937e59c6ab82e257cd926e8f7ae029))
- **service:** Say in the audit what a removed approval rule covered ([55eedd3](https://github.com/AlexGalhardo/stuarty/commit/55eedd379cad127e5d511690bebbfe603d4ce1b0))
- **store:** Order the approval migrations after the projects and the library ([1215a7b](https://github.com/AlexGalhardo/stuarty/commit/1215a7bef66aefef0cf47736c38107c9bf81cc9f))
- **store:** Keep the export batches when the approval rebuild of jobs goes down ([ee713e6](https://github.com/AlexGalhardo/stuarty/commit/ee713e6fc046bd07609cfb5fea76cb86ce09daa5))
- **service:** Never name a zero height for the picture of a very wide image ([b16923e](https://github.com/AlexGalhardo/stuarty/commit/b16923ea95b38c8364d505ec1be2149f0fa3aa4a))
- **ui:** Name the revision of the picture that is drawn on the image canvas ([3b4121d](https://github.com/AlexGalhardo/stuarty/commit/3b4121d7835cb72f7e7bd117a31dfdfcc0dcd7dd))
- **ui:** Take the required slots of a template from the core lint ([e7386f0](https://github.com/AlexGalhardo/stuarty/commit/e7386f0e1643eb4436795a64512473e87d1ed336))
- **web:** Add the command title of keyframe.set_easing in both languages ([d74c892](https://github.com/AlexGalhardo/stuarty/commit/d74c892cdfdbf46f44d79c51b27df5e8eeb240a0))
- **command:** Carry the bezier numbers when text.apply_preset inherits a curve ([2942ef5](https://github.com/AlexGalhardo/stuarty/commit/2942ef510bcbed3d992f7647c7a6c9b99d02503a))
- **core:** Keep the bezier curve of a keyframe through an OTIO round trip ([0bf4959](https://github.com/AlexGalhardo/stuarty/commit/0bf49599ca8bfe73a5cffe12f5e6b3054476be0a))
- **ui:** Show the refusal of a motion preset under its section ([0479915](https://github.com/AlexGalhardo/stuarty/commit/04799151bb961afe8edd7b821a89165843898294))
- **ui:** Label the motion preset defaults without the numbers of one preset ([a643165](https://github.com/AlexGalhardo/stuarty/commit/a6431658184c8591c1bbc4cf29213b1a25af9ba8))
- **ui:** Give the motion preset controls names of their own ([2efd37b](https://github.com/AlexGalhardo/stuarty/commit/2efd37bf32c3b5f937c73db12069e987ec27e976))
- **mcp:** Fit tools/list in its budget after the merge ([1e046ec](https://github.com/AlexGalhardo/stuarty/commit/1e046ecd8cd084c505926681c9afa0d2502727c8))
- **ui:** Render the asset names of the image picker as untrusted text ([28a3e53](https://github.com/AlexGalhardo/stuarty/commit/28a3e5330bb2d9de26137291e9514efba3001276))

### Security

- **security:** Refuse look-alike characters in approval text ([bf3f868](https://github.com/AlexGalhardo/stuarty/commit/bf3f868777059bc7fed492d93ca735e8a4ffb698))
- **security:** Bound the approval queue per submitter and keep a requeued job reserved ([c23220a](https://github.com/AlexGalhardo/stuarty/commit/c23220a8d9a0dc9c15d0ca4c7e92be5a93fce909))
- **security:** Prove a requeued job cannot run past a ceiling it did not reserve under ([b2b56d1](https://github.com/AlexGalhardo/stuarty/commit/b2b56d11268d21a3f5b31024dd809108818f2a7d))
- **security:** Cover the approval threat rows T-48 to T-51 ([d2d0a06](https://github.com/AlexGalhardo/stuarty/commit/d2d0a066b37754888f3ad2d46b81f159096177d0))
- **security:** Name the test of an interrupted run that spent in T-50 ([e2210e9](https://github.com/AlexGalhardo/stuarty/commit/e2210e91ef5e2587d707b445236a5e215d4cddc7))
- **security:** Prove concurrent approvals never pass a cost ceiling ([ceb4175](https://github.com/AlexGalhardo/stuarty/commit/ceb4175b98f38a117b657fbc463b888fb06f0020))
- **security:** Refuse approval writes from a browser session without the launcher secret ([20a3f83](https://github.com/AlexGalhardo/stuarty/commit/20a3f838a1117faa3b0079a607c57d7cd08422ff))
- **security:** Cover a project-limited key and a timeline id (VV-246) ([1148016](https://github.com/AlexGalhardo/stuarty/commit/1148016e0df1d68668bd96a6e6b86c4e92cfb910))
- **security:** Hold a limited key to its project in loadProduction (VV-246) ([c603a02](https://github.com/AlexGalhardo/stuarty/commit/c603a02062ba870e8bf991d92858111d8285db82))
- **security:** Hold a stale timeline key and a key of A in network mode ([fc7afed](https://github.com/AlexGalhardo/stuarty/commit/fc7afedb907061eb90c3e48c65db4e706556f558))
- **security:** State the project limit of a key and list its T-06 tests ([7475ce3](https://github.com/AlexGalhardo/stuarty/commit/7475ce3f5d0e20c2d5a81031d3a71ec8d6e69b3e))
- **security:** Wrap a long line of the network key test for golines ([12a36e6](https://github.com/AlexGalhardo/stuarty/commit/12a36e63b10624991862b83ca8ca50a871d68917))
- **security:** Cover item move, rename, restore and project restore for a limited key ([d42b6fd](https://github.com/AlexGalhardo/stuarty/commit/d42b6fd196134970d19a543c73eb7975774dcbfb))
## [0.31.0] - 2026-11-20

### Added

- **core:** Add the image document, its registry and the structure commands ([9a68ce4](https://github.com/AlexGalhardo/stuarty/commit/9a68ce49675d37d7e86587fd20201caa0d4d3b10))
- **core:** Add the layer transform, set, crop, align, fit, text and shape commands ([dc1985c](https://github.com/AlexGalhardo/stuarty/commit/dc1985ca26e6fcbd73d8201b490356ecff96cfe9))
- **core:** Add undo, redo, conflicts and the diff text of image batches ([ebdca71](https://github.com/AlexGalhardo/stuarty/commit/ebdca71f9cd01a00a2bbb8015f936fbfe097cfb4))
- **export:** Plan an image document and render it through the Go host ([7316350](https://github.com/AlexGalhardo/stuarty/commit/731635085802f25e7eae10cf7a1f7f04a3821855))
- **export:** Encode image exports as PNG and JPEG with presets and a determinism test ([5fbc0de](https://github.com/AlexGalhardo/stuarty/commit/5fbc0de0324424343cf46bbdca7a0ba87c63ba9a))
- **core:** Fill image templates from slot values and lint image documents ([10cf698](https://github.com/AlexGalhardo/stuarty/commit/10cf69848a53b8d5455e381ae1ef0faa63af10d8))
- **service:** Store image documents and serve their create, edit, undo and export ([6159b3a](https://github.com/AlexGalhardo/stuarty/commit/6159b3acaa25e3ed9d2f3030d89f276635542275))
- **web:** Expose the image documents on HTTP, the CLI and the MCP tool with parity ([8cc0ffa](https://github.com/AlexGalhardo/stuarty/commit/8cc0ffaa17a67190b5ac98faac7aa67f5deda2e0))
- **core:** Add the rules of a production ([331d0b1](https://github.com/AlexGalhardo/stuarty/commit/331d0b1f17bb9d9c6b0ff833b7f83ffb83df04f8))
- **store:** Add projects and project_items, rename the table to timelines ([18e05ba](https://github.com/AlexGalhardo/stuarty/commit/18e05ba2dc8da069769bff2abf4dc6e157b14bae))
- **service:** Add the production operations and the timeline_id rule ([2894498](https://github.com/AlexGalhardo/stuarty/commit/289449883282f4e7f88c7ba6d6cb6aafb728a4c3))
- **service:** Export and import the whole project in a bundle ([7ca9d43](https://github.com/AlexGalhardo/stuarty/commit/7ca9d434318dfb5ca7c4e9df4d51a19d42b0a2cf))
- **http:** Expose the production on HTTP, CLI and MCP with the text view ([bf9ace9](https://github.com/AlexGalhardo/stuarty/commit/bf9ace9b914744877896ba892c130daab85cd696))
- **ui:** Build the project overview page and add Duplicate to the card ([0c2b98f](https://github.com/AlexGalhardo/stuarty/commit/0c2b98f1b2a80f007fe3c83d67f83d6fe5f54d15))
- **library:** Add the pure rules of memory, skills and prompts ([c7f6649](https://github.com/AlexGalhardo/stuarty/commit/c7f6649057133bee1e21310bb8d6ddfe4d2dc4f9))
- **library:** Store the library as files and index them for search ([c4a1b44](https://github.com/AlexGalhardo/stuarty/commit/c4a1b441733ac149f2ab7d02eeab9b92586b62f5))
- **library:** Add the rules of memory notes and prompts and the search ([839abab](https://github.com/AlexGalhardo/stuarty/commit/839abab4c64a503043a80b89ebe2713d3bcc2c6a))
- **library:** Write, lint, import, review and export skills ([1b8180e](https://github.com/AlexGalhardo/stuarty/commit/1b8180eb8731702c3e1a025e22b5d54cda05765a))
- **api:** Expose the library on HTTP, the CLI and the MCP tool library ([03c2a43](https://github.com/AlexGalhardo/stuarty/commit/03c2a432ecda1141717c5eb1681bf2ea78dc9a95))
- **ui:** Add the memory, skills and prompts sections of the dashboard ([05cebef](https://github.com/AlexGalhardo/stuarty/commit/05cebef8ca81beb047dc3e9d650d0aecccf79b30))

### Fixed

- **cli:** Use an example host the site check accepts in connection:add ([16089e6](https://github.com/AlexGalhardo/stuarty/commit/16089e6e91b8947c159fbc1b1ce08273edb4f535))
- **core:** End the undo of an image when nothing live remains ([5276bfa](https://github.com/AlexGalhardo/stuarty/commit/5276bfa54e9d8e15659ea6701ded9f63b1ab18ee))
- **export:** Bound the size of an image export and refuse an empty picture ([79eb14f](https://github.com/AlexGalhardo/stuarty/commit/79eb14fabdc1e57b2dfef07b97778c31b2b59cf0))
- **service:** Refuse another user's file, a project-limited key and a chosen id in image operations ([a11726a](https://github.com/AlexGalhardo/stuarty/commit/a11726a4d214458f188cb7e75d099ad0d253fb6f))
- **service:** Name an image from the project entry of the caller, not the global row ([d75e020](https://github.com/AlexGalhardo/stuarty/commit/d75e020e8e96efe0dc030f8c578bf82a5a234bc5))
- **service:** Never fall back to the global file name in network mode ([c1cf5b0](https://github.com/AlexGalhardo/stuarty/commit/c1cf5b00c6977356cac5b6591d9683fe87bc5733))
- **service:** Consume an image undo step whose primitives were all skipped ([51999ff](https://github.com/AlexGalhardo/stuarty/commit/51999ffde85f2de7eed044688d5420583420cbfa))
- **export:** Upload each layer of an image as soon as it is drawn ([21bb217](https://github.com/AlexGalhardo/stuarty/commit/21bb2170104f67510d4d5a689063e0f265ab9ec6))
- **image:** Quote untrusted names and texts in the image views ([f3ac9ad](https://github.com/AlexGalhardo/stuarty/commit/f3ac9adf002d02daac5bb367bbefb79df1ee2cdb))
- **service:** Check the quality of an image export that names a preset ([2779add](https://github.com/AlexGalhardo/stuarty/commit/2779add9e5eaa094593c80fa4e2c673209702657))
- **store:** Rename the image migration to avoid epic 04's timestamp ([b2655bc](https://github.com/AlexGalhardo/stuarty/commit/b2655bc9b821b06494db015c4f3f61c510263904))
- **mcp:** Shorten tool descriptions so tools/list fits NFR-020 with 18 tools ([16ede9f](https://github.com/AlexGalhardo/stuarty/commit/16ede9ff4acb3a9f540819de75aa35e090c0e7a4))
- **store:** Derive the item id of a migrated timeline from the project id ([3a44916](https://github.com/AlexGalhardo/stuarty/commit/3a449164e47a7a573dad2e8c3e2148de6e7e7887))
- **ui:** Run the onSelect of a menu item when it is chosen ([e1b757b](https://github.com/AlexGalhardo/stuarty/commit/e1b757baf5c898a31fbf19a3a2dc01ea89231f00))
- **core:** Keep the brief out of the trash and bound the items of a project ([3203ebb](https://github.com/AlexGalhardo/stuarty/commit/3203ebb1df728b72a7890c2b81533cd935f273be))
- **service:** Carry the texts of a bundle in entries of their own ([2c5ae33](https://github.com/AlexGalhardo/stuarty/commit/2c5ae33bec15a75981ccb39d71dd9264c03b7f13))
- **ui:** Open the first active timeline from the project card ([046ec4c](https://github.com/AlexGalhardo/stuarty/commit/046ec4c180489af9a09f2432b7e914c449480ee4))
- **ui:** Name the timeline of the editor in the body of a JSON request ([7479d9e](https://github.com/AlexGalhardo/stuarty/commit/7479d9e5398200daed1e8fbdb0bfa82e609485b0))
- **library:** Look for hidden text in every text file of a skill ([37d98c7](https://github.com/AlexGalhardo/stuarty/commit/37d98c74ab66bdd8de42c64affb72ed3dfd9d6ab))
- **library:** Keep what a write does not carry and refuse oversized folder files ([73773d3](https://github.com/AlexGalhardo/stuarty/commit/73773d3dfa72c4988813e0771e60d5994c0f9650))
- **library:** Write the notes of a project key into its project by default ([54f4b3c](https://github.com/AlexGalhardo/stuarty/commit/54f4b3c497c774f13517b7050ca5ddf56134306f))
- **library:** Seal skill reviews, bind them to the text read and delimit file names ([eebaf13](https://github.com/AlexGalhardo/stuarty/commit/eebaf131ff1dfb502f199797f8f7e9d00d2b1433))

### Security

- **security:** Add the image operations to the scope matrix ([dc96256](https://github.com/AlexGalhardo/stuarty/commit/dc96256a4576b7c8edbc05640efbe1b31e5293bb))
- **security:** Reproduce VV-243 metadata dial and echoed secret ([bd23242](https://github.com/AlexGalhardo/stuarty/commit/bd23242120bc6fe42d51a9d61b4393c0053fee9d))
- **security:** Refuse named cloud metadata addresses and scrub an echoed secret ([c8f2b61](https://github.com/AlexGalhardo/stuarty/commit/c8f2b61aff00551b04e0765c579991b48b8b99fe))
- **security:** Expect the dropped sentence when a destination echoes the credential ([4d04e9e](https://github.com/AlexGalhardo/stuarty/commit/4d04e9ea4b090353ba33e6835edd02be5e5ac7df))
- **security:** Wrap a long line of the echo test ([a901ee5](https://github.com/AlexGalhardo/stuarty/commit/a901ee583144cb315519edb90a5a5545c00c5c33))
- **security:** Refuse a metadata address with a zone or behind NAT64 ([da7bf7f](https://github.com/AlexGalhardo/stuarty/commit/da7bf7f18de46b00a400323f92a208bd6b903aef))
- **security:** Find a JSON escaped or upper hex echo of the credential ([5944e50](https://github.com/AlexGalhardo/stuarty/commit/5944e5033d2afee0214f16375c385bc581bcd456))
- **security:** Describe the JSON escapes of the scrub in words ([78669df](https://github.com/AlexGalhardo/stuarty/commit/78669df19362b37d681a91e56c15d3b20a7e76e8))
- **security:** Tell no timeline existence to a caller who may not reach it ([516d780](https://github.com/AlexGalhardo/stuarty/commit/516d780b55be9b84b00336f860742d1b3b64a02f))
- **security:** Reproduce embedded IPv4 dials and a mid-secret echo ([061a70a](https://github.com/AlexGalhardo/stuarty/commit/061a70a06cb4c70b83da9970ed715a5c1f917da0))
- **security:** Size the mid-secret echo fixture to 40 bytes ([0547321](https://github.com/AlexGalhardo/stuarty/commit/0547321f7a367e171946dfd4073b06e03afc8f8e))
- **security:** Judge embedded IPv4 at dial and scrub any echo window ([2626bb4](https://github.com/AlexGalhardo/stuarty/commit/2626bb416fb1279413c36725177ffe7ddf111a63))
- **security:** Find base64 of any part of the secret and bound the scrub cost ([e431aaf](https://github.com/AlexGalhardo/stuarty/commit/e431aafdd4f8912b963e180b2e0d3d6145267c24))
- **security:** Document the tunnel forms the dial gate leaves undecoded ([9496918](https://github.com/AlexGalhardo/stuarty/commit/9496918ec63ea3bedf1e908d75f6b93112b1e799))
- **security:** Wrap two long lines of the scrub for golines ([7d6be26](https://github.com/AlexGalhardo/stuarty/commit/7d6be26be540649242d89f6514153bbdca1d3ea2))
- **security:** Cover the library threats T-44 to T-47 ([13d1703](https://github.com/AlexGalhardo/stuarty/commit/13d17033116dc229e288c078174a4c578ab956ac))
## [0.30.0] - 2026-11-20

### Added

- **desktop:** Redraw the desktop editor with gio ([f14a062](https://github.com/AlexGalhardo/stuarty/commit/f14a062b685f982379cc54706d2dfe1eccd053b7))
- **desktop:** Play the preview from a live decode ([a9488e1](https://github.com/AlexGalhardo/stuarty/commit/a9488e119436443e2613f52c24b4c4d9498dd463))
- **desktop:** Add a light theme and remember it ([39a2157](https://github.com/AlexGalhardo/stuarty/commit/39a21577a8165bd6dfdc27aee1a72d468cfb47cb))
- **ui:** Match the desktop palette with Base UI primitives ([d316d74](https://github.com/AlexGalhardo/stuarty/commit/d316d749c68ca1c52116b5ba4914c7ca9f457495))
- **service:** Read the former VUKEM_VIDEO_* variables with one warning ([552d8c8](https://github.com/AlexGalhardo/stuarty/commit/552d8c82da33733c2b59a6d7ac536c4103e7403a))
- **workspace:** Adopt the workspace of Vukem Video at the first start ([a6f7745](https://github.com/AlexGalhardo/stuarty/commit/a6f7745110d02c6aa393af996c37d1cbfd3c2016))
- **ui:** Add the dashboard layout with the sidebar, the profile and the theme toggle ([58e2865](https://github.com/AlexGalhardo/stuarty/commit/58e2865056972b2e1dad1f9bd1da0d02a7cb57b8))
- **ui:** Add the dashboard home and the section routes with their empty states ([7cfa7be](https://github.com/AlexGalhardo/stuarty/commit/7cfa7bed0b7dd48a5cdf8bc9e4ffa08142bd039f))
- **ui:** Add the dashboard link to the editor top bar and the navigation spec ([69ddc6f](https://github.com/AlexGalhardo/stuarty/commit/69ddc6fc0d0b6c8b7f3e6b2aaa03ad49566faa1b))
- **ui:** Add the render worker and its protocol over the core module ([39d9d2e](https://github.com/AlexGalhardo/stuarty/commit/39d9d2e34c24f869f5aeebf77c50d3687ea46963))
- **ui:** Draw the preview with the renderer of the export in a worker ([48b6c94](https://github.com/AlexGalhardo/stuarty/commit/48b6c94248a33dce3de9f7885658307eeeb7e801))
- **service:** Add connections, their table and the service operations ([6c90e95](https://github.com/AlexGalhardo/stuarty/commit/6c90e952a7ce32f274096ca6ba569340dd8f1c7c))
- **api:** Expose connections on HTTP, the CLI and MCP ([6310307](https://github.com/AlexGalhardo/stuarty/commit/6310307de1d29c8a5791bec6091d24fc887a611d))
- **ui:** Add the Connections page ([527a58a](https://github.com/AlexGalhardo/stuarty/commit/527a58af3f4a11d1be37d294f14c73ba1752ceb7))

### Fixed

- **ci:** Clear the checks blocking release 0.29.0 ([bbf3ab6](https://github.com/AlexGalhardo/stuarty/commit/bbf3ab6568e3bfb4e23db867755facb0453f3208))
- **ui:** Let the preview follow the page theme ([daa8682](https://github.com/AlexGalhardo/stuarty/commit/daa8682baa7cbfda95c35bbb9b316ad2e18aa5d0))
- **workspace:** Ask to close Vukem Video instead of reporting an internal error ([015f1c8](https://github.com/AlexGalhardo/stuarty/commit/015f1c8c577e70373feb587b704c4c9430fc442a))
- **desktop:** Confirm an update from Vukem Video in its own state folder ([1dec0a2](https://github.com/AlexGalhardo/stuarty/commit/1dec0a2be576cb7468fd54247a87b95ea5604e77))
- **installers:** Upgrade an installed Vukem Video instead of leaving it running ([b9fec27](https://github.com/AlexGalhardo/stuarty/commit/b9fec27eac182a991848a7b5921169de2ec47407))
- **workspace:** Answer an adoption failure with its reason ([e688fdf](https://github.com/AlexGalhardo/stuarty/commit/e688fdfdccd52c93a2e3c0a1a37926c6186abfb7))
- **workspace:** Report a failed note after a verified copy as adopted ([b60fc92](https://github.com/AlexGalhardo/stuarty/commit/b60fc92a21a349edb7a2ee7c03a2ef27473c4ce8))
- **desktop:** Show the adoption of the Vukem Video workspace in a dialog ([7b0e607](https://github.com/AlexGalhardo/stuarty/commit/7b0e607ac04afbb3d185caacee0915cff00ec638))
- **service:** Say where to put a former variable that the new name overrides ([dfb9393](https://github.com/AlexGalhardo/stuarty/commit/dfb939343610ebeda6223af4eab1e19dea2d93f4))
- **ui:** Keep the pages in the dashboard at 320 px and let an anchor button be a link ([2579c59](https://github.com/AlexGalhardo/stuarty/commit/2579c596577e51c833c404aaed8580846d2a7403))
- **ui:** Keep the button role of an asChild button that is not a link ([5c7e117](https://github.com/AlexGalhardo/stuarty/commit/5c7e11774a04dde14384b3593e8a90968c39a489))
- **ui:** Make the sidebar strip accessible and keep every control in a landmark ([979ae65](https://github.com/AlexGalhardo/stuarty/commit/979ae65d7ca53c17bc25962b8cbfc5c75266ac5f))
- **ui:** Copy an odd crop of a decoded frame from even pixels ([8c148e6](https://github.com/AlexGalhardo/stuarty/commit/8c148e6b7831706d59d8d0055685e09aebba3e29))
- **preview:** Start a new renderer instance after a trap ([49602a0](https://github.com/AlexGalhardo/stuarty/commit/49602a0f79a887e10749dd44cf8aaabac8a30f6d))
- **ui:** Read the forced preview executor through prefs ([e4fec44](https://github.com/AlexGalhardo/stuarty/commit/e4fec44605155bc5bfb70aee2deadf3b0bfad5bd))
- **ui:** Let the render worker call fetch and nothing else of R6 ([b10fe40](https://github.com/AlexGalhardo/stuarty/commit/b10fe409646820c76506994cefaf9f570bc4da15))
- **preview:** Deliver layers at the size the export does and restore the gate ([2e0c9ec](https://github.com/AlexGalhardo/stuarty/commit/2e0c9ecae6b66169f570502b87a71c3bd9907390))
- **preview:** Draw a frame of playback once with the renderer ([9c75031](https://github.com/AlexGalhardo/stuarty/commit/9c7503193caf4f321f74187dcd7b0deaa7f75003))
- **service:** Keep the doctor vault check safe without a registry ([de65008](https://github.com/AlexGalhardo/stuarty/commit/de65008b9d454b878ad1720af2be8f5561171717))

### Security

- **repo:** Close the gaps in the project permission rules ([715f276](https://github.com/AlexGalhardo/stuarty/commit/715f27645884b5b3a9abd7f5bf8df72233275d0c))
- **security:** Allow WebAssembly compilation for the render worker ([420f641](https://github.com/AlexGalhardo/stuarty/commit/420f64176afae9b52ff180b0b5bbc122118d5cf8))
- **security:** Add the encrypted secret vault ([792c9a0](https://github.com/AlexGalhardo/stuarty/commit/792c9a0c1c190e0569b46dd0d0272c22f87ea8a5))
- **security:** Add the master key providers of the vault ([232f8f0](https://github.com/AlexGalhardo/stuarty/commit/232f8f06f57817ddbf1faae80cfaf6fb05f0196c))
- **security:** Add the outbound rule check and the T-38 to T-41 tests ([654a821](https://github.com/AlexGalhardo/stuarty/commit/654a8214a0ec95d806361141719b555495f6521c))
- **security:** Keep secrets out of the bundle, the logs and the answers ([3a2e776](https://github.com/AlexGalhardo/stuarty/commit/3a2e776c353abc169a8fc778c14ca893eb606e5e))
- **security:** Report the vault in doctor and stop handing the vault out ([19c1159](https://github.com/AlexGalhardo/stuarty/commit/19c115988972ad405f633bc5b0a4b5602369536e))
- **security:** Make the outbound rule see TLS dials and DNS lookups ([0f0c61d](https://github.com/AlexGalhardo/stuarty/commit/0f0c61d138b394b353849faab26dd0463d765ce1))
- **security:** Check the modes of the vault and the key file on Unix ([2ea2526](https://github.com/AlexGalhardo/stuarty/commit/2ea2526dd1c87c37a503499508ada31a0041b428))
## [0.29.0] - 2026-11-18

### Added

- **ui:** Build shared primitives on Base UI and UI SDK ([3be3e12](https://github.com/AlexGalhardo/vukem-video/commit/3be3e1297c4b1b2cc832e272ec3aaf979c0ba1c2))

## [0.28.0] - 2026-11-17

### Added

- **ui:** Set a transparent project background ([66bb72b](https://github.com/AlexGalhardo/vukem-video/commit/66bb72bbe4a6f371a725963ece586e5e60c48587))
- **engine:** Crop each export frame when the crop changes ([0f56e5a](https://github.com/AlexGalhardo/vukem-video/commit/0f56e5a5363ea60c6e9769c1e4d870448111e425))
- **engine:** Draw a clip shadow and outline ([1e71b5d](https://github.com/AlexGalhardo/vukem-video/commit/1e71b5dd22cd20f8e1227e359a5752d01714c182))
- **engine:** Keep uncovered pixels transparent in GIF and PNG ([bd34956](https://github.com/AlexGalhardo/vukem-video/commit/bd34956a49d6cd7ce7192bf1329185e46326897a))
- **ui:** Draw a clip shadow and outline in the preview ([0a42e2b](https://github.com/AlexGalhardo/vukem-video/commit/0a42e2b55c89585737b80a592fd3b1890c57f0d1))
- **ui:** Show the release version at the bottom left ([99182f9](https://github.com/AlexGalhardo/vukem-video/commit/99182f9d1162388880ed54707347be01c0f8f089))
- **installers:** Name downloads with the dashed release version ([5b6bae6](https://github.com/AlexGalhardo/vukem-video/commit/5b6bae6484683d5de563c19fb196df4fad35a8ea))

### Fixed

- **repo:** Check a nested Go module inside its own module in the verify skill ([26a4ea4](https://github.com/AlexGalhardo/vukem-video/commit/26a4ea4c0423dcbe13834f77aeb6d5e96ddea45e))
- **service:** Return E_FFMPEG_MISSING when ffprobe is absent ([e17c50f](https://github.com/AlexGalhardo/vukem-video/commit/e17c50f43023e1f8ff1b31279a9d798acd248c47))
- **ui:** Send ripple track when Q or W trims to the playhead ([17482d3](https://github.com/AlexGalhardo/vukem-video/commit/17482d340e0bd3719cdb0216f92a71af28403d28))
- **ui:** Draw the per-frame mask and crop in the preview ([e301375](https://github.com/AlexGalhardo/vukem-video/commit/e3013758372f666dcd44fead1ca484a74bf9c881))
- **service:** Truncate values quoted by validation errors ([c694faa](https://github.com/AlexGalhardo/vukem-video/commit/c694faab413385b22bb1d3ee9970f8e9db71f262))
- **cli:** Keep the import hint when a wait redraws the text ([ccf4989](https://github.com/AlexGalhardo/vukem-video/commit/ccf4989a68cfa3c353e93c5c431ff19e6baa1e37))
- **cli:** Do not repeat a flag value in a validation error ([01996fb](https://github.com/AlexGalhardo/vukem-video/commit/01996fb25ae317c47cf45aa226e89b76876d0f7d))
- **command:** Name the target before a missing resolved argument ([4172545](https://github.com/AlexGalhardo/vukem-video/commit/4172545f4f4697b7294dcbaa2304dc3ce1a0116c))
- **http:** Reject dry_run text that would apply a write ([e9b1ad7](https://github.com/AlexGalhardo/vukem-video/commit/e9b1ad727aee963ca64d100438a57428dc002dbd))
- **engine:** Let overlapping ffmpeg runs finish ([8ba0c50](https://github.com/AlexGalhardo/vukem-video/commit/8ba0c5052a98270ac43b911e02ad7344f18f985a))
- **engine:** Satisfy the linter on the scaled deadline and the shadow blend ([157d351](https://github.com/AlexGalhardo/vukem-video/commit/157d351a6f878188a32ffd57931cb3505812ec7d))
- **ui:** Crop a still preview to the frame crop ([f91f1cb](https://github.com/AlexGalhardo/vukem-video/commit/f91f1cbe9e6d699efe9c0f80b4ad3d2b714552af))

## [0.27.0] - 2026-11-17

### Added

- **ui:** Word the server errors in the language of the page from their code and details ([d81ed76](https://github.com/AlexGalhardo/vukem-video/commit/d81ed76bc744369d4f07300b77f7b7dd2c8d0d2c))
- **ui:** Choose the destination track of Add to timeline ([2dcb3a5](https://github.com/AlexGalhardo/vukem-video/commit/2dcb3a5e7ef5117b7947438fa6355518740d84a2))
- **ui:** Shrink the accessibility tree of the editor for agents that read it ([7e9833a](https://github.com/AlexGalhardo/vukem-video/commit/7e9833ab1ab28908d48608f57db53dd6fe6cb8a8))
- **core:** Keyframe the mask and the crop sides of an item ([9e41cc4](https://github.com/AlexGalhardo/vukem-video/commit/9e41cc4f7900356fa5f5562e4b50e8765f501912))
- **core:** Accept a transparent project background ([9db7dc8](https://github.com/AlexGalhardo/vukem-video/commit/9db7dc8c0018e21cb94132927a471c57a82ffacb))
- **core:** Add a drop shadow and an outline to clips ([e454682](https://github.com/AlexGalhardo/vukem-video/commit/e454682ed80062ab8d9f336e394beff02ff72a3b))
- **core:** Repeat an item back to back with item.repeat ([c39e300](https://github.com/AlexGalhardo/vukem-video/commit/c39e30083389f374e22e993821ad09fca317f6fa))
- **ui:** Library tab rail and media panel ([faa0e41](https://github.com/AlexGalhardo/vukem-video/commit/faa0e412842a36c146f6d79d0bea8518d5beeb11))
- **ui:** Audio and text panels ([835d41b](https://github.com/AlexGalhardo/vukem-video/commit/835d41bc70fcbdf6052d4bafd96014523d503027))
- **ui:** Captions panel ([1629ebd](https://github.com/AlexGalhardo/vukem-video/commit/1629ebd777a3331f92de99ec79e925e38a833ddb))
- **ui:** Transitions and filters panels ([4577514](https://github.com/AlexGalhardo/vukem-video/commit/4577514166e96af0fc8cd9f36d05a49225eeb4ea))
- **ui:** Templates panel ([f44dcd0](https://github.com/AlexGalhardo/vukem-video/commit/f44dcd04c8af8ef3fccd1af9519be286b854842d))
- **ui:** Effects panel ([8cf11aa](https://github.com/AlexGalhardo/vukem-video/commit/8cf11aa9c2240293b1d1fda928d446b4d2766828))
- **ui:** Inspector tabs ([23b18c7](https://github.com/AlexGalhardo/vukem-video/commit/23b18c793aae4e5dfe8e122318a1030d2ca3f472))
- **ui:** Keyframe the mask box, feather and crop sides ([318c577](https://github.com/AlexGalhardo/vukem-video/commit/318c5772c07e41f416f71acbb84104b468f0f8f4))
- **ui:** Add the Shadow and Outline sections and the Repeat item action ([30140ef](https://github.com/AlexGalhardo/vukem-video/commit/30140effa32d3bb450e1a4125204a1809977b1d7))
- **ui:** Read the per-frame masks and crops of a plan layer ([5124d20](https://github.com/AlexGalhardo/vukem-video/commit/5124d2045cd21424ffd5047aa6c2562f3baf9ab1))
- **mcp:** Accept detail document in project_get, as the API and the CLI do ([bc906d6](https://github.com/AlexGalhardo/vukem-video/commit/bc906d6b91fb379e02391cd91308b74e577c5856))
- **ui:** Timeline toolbar tools ([10e671d](https://github.com/AlexGalhardo/vukem-video/commit/10e671d9155038c6186ab87857dbc50d30c13cc6))
- **ui:** Player row controls ([e25a8e0](https://github.com/AlexGalhardo/vukem-video/commit/e25a8e060fce5e33e4c48bc6b439a278c3ac1161))
- **ui:** Editor menu and clip context menu ([0c3708d](https://github.com/AlexGalhardo/vukem-video/commit/0c3708d394337311823fec2e526ffa14647c34d8))
- **ui:** Projects grid ([3e7cd7f](https://github.com/AlexGalhardo/vukem-video/commit/3e7cd7ff9aa50116d029014bde37648074bffaa2))
- **ui:** Shortcuts dialog footer ([55fb22f](https://github.com/AlexGalhardo/vukem-video/commit/55fb22fe01b4d6465b05450f00356f3b6d3edd97))
- **desktop:** Native menu bar ([01725f7](https://github.com/AlexGalhardo/vukem-video/commit/01725f73db9b02ba6dc0ae19f6b87d367198bfb4))

### Fixed

- **mcp:** Answer an unknown method of POST /mcp with a JSON-RPC -32601 error ([3f3de93](https://github.com/AlexGalhardo/vukem-video/commit/3f3de93961ccda9b889de9ab3dab03bb4032499c))
- **mcp:** Accept a numeric fps in project_create as the CLI and the API do ([ccbd3d4](https://github.com/AlexGalhardo/vukem-video/commit/ccbd3d413ff2c3996e73ceb74e83db02b54aba56))
- **service:** Mark the lint of an edit result as lint_scope fast ([28e7496](https://github.com/AlexGalhardo/vukem-video/commit/28e7496bf9a5984a2fc95b56cdd22a1817905805))
- **service:** Report media matched by name on OTIO import and keep track lock, solo and volume ([788ffd2](https://github.com/AlexGalhardo/vukem-video/commit/788ffd2690f80a91656779fc380965125b49b59a))
- **core:** Do not echo a long value back in an error message ([874ded1](https://github.com/AlexGalhardo/vukem-video/commit/874ded1be4ee774a3350dc6a6e13db7f9c88f62b))
- **service:** Name the job_get tool in the text of a job relayed over HTTP ([7566474](https://github.com/AlexGalhardo/vukem-video/commit/75664747c3489e2a8793dc92fbd694bbddf74d9a))
- **http:** Refuse a limit that is not an integer on GET /assets ([4d0b08a](https://github.com/AlexGalhardo/vukem-video/commit/4d0b08afef4a91df9b728cbee387538587b17054))
- **http:** Declare the 200 of a media import with a refused file and clear deduplicated on it ([823a696](https://github.com/AlexGalhardo/vukem-video/commit/823a69682733707664d575a8ee0fe6ec979bfe3a))
- **service:** Say that until is the missing parameter of a plan window ([657db90](https://github.com/AlexGalhardo/vukem-video/commit/657db90b029e02a2b0ca79bc141e754e2e4f8c06))
- **core:** Keep hints whole and quote up to 256 characters of a value ([6672696](https://github.com/AlexGalhardo/vukem-video/commit/66726968f106a52fede98b2d26318693b6ec2c73))
- **core:** Name the call that makes the speech in the audio.duck hint ([83e7a97](https://github.com/AlexGalhardo/vukem-video/commit/83e7a9792984292363e147f52dada71c8f243658))
- **service:** List the reference of a media matched by name in the provenance table ([b247511](https://github.com/AlexGalhardo/vukem-video/commit/b247511b7c311036257cd4a8232dfaf1bb487b68))
- **core:** Count every copy of a caption against the document word limit in item.repeat ([d4ea79f](https://github.com/AlexGalhardo/vukem-video/commit/d4ea79ff6688eae81597f236cc5ea6f563fef12d))
- **core:** Do not flag a fade out at the end of the picture when a title or bar runs on ([aa182ab](https://github.com/AlexGalhardo/vukem-video/commit/aa182abdb0d65d1b432dc30d2cc3f9e6665e36f5))
- **core:** Move the timeline markers with a ripple all ([95ef2ca](https://github.com/AlexGalhardo/vukem-video/commit/95ef2ca171d8a51a3e0ea09b03e7823b9bfb455d))
- **ui:** Name the shadow, outline and repeat commands in the palette ([003b539](https://github.com/AlexGalhardo/vukem-video/commit/003b5394403f89d6acd3c4a99ad56520d5ef066f))
- **service:** Refuse a caller-supplied resolved argument on analysis commands ([994af21](https://github.com/AlexGalhardo/vukem-video/commit/994af2163021d33ae408bc89ac7cee20d6d91fd4))
- **http:** Refuse a text dry_run in a JSON body ([e66cb19](https://github.com/AlexGalhardo/vukem-video/commit/e66cb1990d96c70c0bc07cab036f10cf9083d40f))
- **api:** Declare the dry-run 200 of jobs.start and the 409 of rev reads ([fb0d06e](https://github.com/AlexGalhardo/vukem-video/commit/fb0d06e60a18d6428f7b86ca14a6e02d183cdfdc))
- **http:** Check dry_run of undo and redo in the binding, not in a middleware ([7e473fb](https://github.com/AlexGalhardo/vukem-video/commit/7e473fb36949774a7b3ae7fe03041eeaf34f431c))
- **service:** Report a cancelled plan window as client closed, not internal ([29cc698](https://github.com/AlexGalhardo/vukem-video/commit/29cc698e0bb8e189d78f4c075e499279cbec25b0))
- **mcp:** Answer the requests already read when stdin closes ([4999b95](https://github.com/AlexGalhardo/vukem-video/commit/4999b958acdb573d11bc6198e294a2ab6a4eb2da))
- **mcp:** Mark media_import as an error when nothing was imported, keep the hint ([d93289e](https://github.com/AlexGalhardo/vukem-video/commit/d93289e82232da911231ae0ef13cdc2c05685967))
- **cli:** Give each doctor check a status field that matches the text ([d36d528](https://github.com/AlexGalhardo/vukem-video/commit/d36d52811607fe6d8eb7df67d9a239ee61950802))
- **repo:** Read the added test lines in one pass and ignore timeout limits in the flaky check ([6b3e1ee](https://github.com/AlexGalhardo/vukem-video/commit/6b3e1ee129316b91d81ab9fa1fc1bd7e1030eb06))

## [0.26.0] - 2026-11-17

### Added

- **api:** Answer the content hash of an import and list the workspace files by hash ([dc787fa](https://github.com/AlexGalhardo/vukem-video/commit/dc787fa7115c95553901e9b1576fe63def161778))
- **ui:** Choose the brand kit logo from the images of the workspace ([2641ab0](https://github.com/AlexGalhardo/vukem-video/commit/2641ab07f0991f890ce62e8e32a3f4a4234c5d6d))
- **ui:** Pick an imported font file in the editor Fonts section ([8b3dbb9](https://github.com/AlexGalhardo/vukem-video/commit/8b3dbb987b6d2614c37b0f7a61fc14fe5f3c090a))
- **ui:** Show the transcription helper state and an install button in Settings ([5318a5a](https://github.com/AlexGalhardo/vukem-video/commit/5318a5ad499b47714f15c0f6039cf5a22141c5cc))
- **core:** Let audio.mark_beats take a manual tempo when no pulse is found ([6f64e8e](https://github.com/AlexGalhardo/vukem-video/commit/6f64e8e7833261b18f4ae66b0e11c3cd4250f04c))
- **core:** Make start optional on clip.add and audio.add ([17ed8bf](https://github.com/AlexGalhardo/vukem-video/commit/17ed8bfa68d0d5b14f6e242632d5cc8164fdb43c))
- **ui:** Add a chroma key eyedropper that picks the colour from the preview ([288b8ee](https://github.com/AlexGalhardo/vukem-video/commit/288b8ee3841cfb0e064a960200a6da4ffd38aaec))
- **ui:** Hint under the text font select where to bring in brand kit fonts ([5aa59fb](https://github.com/AlexGalhardo/vukem-video/commit/5aa59fb689bb87905d01079274bdd040cfca6d9e))

### Fixed

- **service:** Refuse an OTIO import with missing media and accept the saved export answer ([475af46](https://github.com/AlexGalhardo/vukem-video/commit/475af46e7fd8f92687a904d28e37161fc457ccfe))
- **cli:** Doctor says it ran no check, names the Windows limits and ignores optional helpers ([7595f60](https://github.com/AlexGalhardo/vukem-video/commit/7595f602519c689b0a0cb9a08573832a6f3d389a))
- **api:** Add Retry-After to the refusal of an export when all are busy ([354e09c](https://github.com/AlexGalhardo/vukem-video/commit/354e09ca969a2867d23dc486c4bcc16026e21993))
- **api:** Report the workspace folder to a local editor session ([da7e971](https://github.com/AlexGalhardo/vukem-video/commit/da7e971aba0aac8a95c7b53bdad6a4bea38a7455))
- **service:** Answer the existing template when the same file is imported again ([11bbc5b](https://github.com/AlexGalhardo/vukem-video/commit/11bbc5b5a2494f7924a098de6aa5c121df1c47e3))
- **cli:** Keep SKILL.md under its size limit after the otio:import flag ([0eaa961](https://github.com/AlexGalhardo/vukem-video/commit/0eaa961b0f8499b0384dba17b386fd23e93fad9c))
- **service:** Show one asset duration in the probe, media.list and events ([7c911f4](https://github.com/AlexGalhardo/vukem-video/commit/7c911f43868d35104dee26eb5acf2bcc316604df))
- **service:** Doctor warns when the resolved FFmpeg is GPL ([ef3ca0c](https://github.com/AlexGalhardo/vukem-video/commit/ef3ca0c198806dcc1b92a7b65f20267aabbb4708))
- **ui:** Say when a template file was already imported ([d3cb486](https://github.com/AlexGalhardo/vukem-video/commit/d3cb486c316d93cb5c4ec6fc135b0e00f5899015))
- **core:** Name the unsupported unit when a time like 800ms is rejected ([bb5ca9d](https://github.com/AlexGalhardo/vukem-video/commit/bb5ca9d16b0d489de35c37f4b800acdcf2cfb79b))
- **desktop:** Read the WebView2 variables before the Wails loader blanks them ([62ef199](https://github.com/AlexGalhardo/vukem-video/commit/62ef19929808c0d2a86c26882da664d0cef89c11))
- **desktop:** Say what to do when a server already runs on the workspace ([8494fbe](https://github.com/AlexGalhardo/vukem-video/commit/8494fbe9f16d0af0945e5e728a132bc0ce4475eb))
- **http:** Refuse a non-JSON body on POST /system/launch ([facb251](https://github.com/AlexGalhardo/vukem-video/commit/facb251082a10b569a7d3d010b6acf0da5f40372))
- **api:** Declare the 403, 404, 429 and 200 statuses the routes return in the OpenAPI document ([d665110](https://github.com/AlexGalhardo/vukem-video/commit/d66511085f901c2e9eea582fd876ad7aacade91a))
- **service:** Show the colour grade and the LUT of a clip in the project summary ([32d8f4f](https://github.com/AlexGalhardo/vukem-video/commit/32d8f4f41f4d22725c56a5dfb16ccea577336903))
- **mcp:** State the bound of a failed numeric, length, item or pattern constraint ([491a89d](https://github.com/AlexGalhardo/vukem-video/commit/491a89d8c3440498aa909fdf8cf317a55ea5c55f))
- **cli:** Name the workspace a not-found answer read when no server owns it ([3be0028](https://github.com/AlexGalhardo/vukem-video/commit/3be0028bfb6af6081c06b9964cc4174030e2bc2c))
- **http:** Tell a browser with no session how to sign in to the local editor ([ac56454](https://github.com/AlexGalhardo/vukem-video/commit/ac56454cdf81ea8432e145bf8086b011b340da2e))
- **ui:** Add a keyframe toggle to each animatable Transform row ([4fe620e](https://github.com/AlexGalhardo/vukem-video/commit/4fe620e70a69b3378786a8da5507cb94bb3779d7))
- **ui:** Keep \<html lang> in sync with the active language on Inertia pages ([9c326a8](https://github.com/AlexGalhardo/vukem-video/commit/9c326a88965e3d184b0e66a482ce10406a894510))
- **ui:** Give the keyframe row inputs room so axe target-size passes ([fd0d5ca](https://github.com/AlexGalhardo/vukem-video/commit/fd0d5cab5ebe5fc03d0e70a4894ff5261e411718))
- **core:** Keep captions linked to the pieces that clip.remove_silences makes ([60d8cc8](https://github.com/AlexGalhardo/vukem-video/commit/60d8cc88aa1a669030006ebd10339e97d6d94dba))
- **core:** Report a bad speed curve point at args.curve[i].speed ([518adb5](https://github.com/AlexGalhardo/vukem-video/commit/518adb56f298aae486ddd5e6e4520d7e7dec9d07))
- **core:** Report the new duration and the gap when clip.set_speed shortens a clip ([418c3ce](https://github.com/AlexGalhardo/vukem-video/commit/418c3ce695d01781888e8773969d928823ae6ef5))
- **service:** Let audio.mark_beats with a manual tempo skip the beat grid of the asset ([f1351b8](https://github.com/AlexGalhardo/vukem-video/commit/f1351b844d9d449645e4624d39d38a1af5d96290))
- **core:** Make the E_OVERLAP hints say what replace removes and offer next_free ([5c5a683](https://github.com/AlexGalhardo/vukem-video/commit/5c5a683b68faa83e30a1c4d5e7651eed1f613f0c))
- **ui:** Omit start on Add to timeline so the item lands after the last one ([f6021c9](https://github.com/AlexGalhardo/vukem-video/commit/f6021c9bc4ffa6813c756ce0936ff975d1e1ea15))
- **engine:** Accept a weaker tempo peak so music with a busy top end gets a beat grid ([d03271b](https://github.com/AlexGalhardo/vukem-video/commit/d03271b185f695e1f09542b571ad24daf5cb3fd5))
- **service:** Report imported and failed counts for a partial media import ([878359b](https://github.com/AlexGalhardo/vukem-video/commit/878359be71873dfe08bce5e42d70dcef0037c750))
- **engine:** Record has_alpha in the facts of an image with an alpha channel ([5a25048](https://github.com/AlexGalhardo/vukem-video/commit/5a250484d4916888f845e0641004d3b13b11f042))
- **service:** Name the tools, not HTTP routes, in the hints a client relays ([a5bd9d8](https://github.com/AlexGalhardo/vukem-video/commit/a5bd9d89cba113c08b7bba8514360f5491f92ddd))
- **service:** Do not download a helper that is installed and verified ([431da46](https://github.com/AlexGalhardo/vukem-video/commit/431da46b55e6445b9b11e937f4c1e8b2c1a4209c))
- **http:** Name the local session cookie after the port of the server ([5454848](https://github.com/AlexGalhardo/vukem-video/commit/545484820712088038f3cc01bb2d6942088cd93f))
- **service:** Answer E_VALIDATION for a negative snap-points revision ([43f0e07](https://github.com/AlexGalhardo/vukem-video/commit/43f0e075bec513244fb442894c71ce5b2874fe08))
- **mcp:** Name the missing property when a required argument is absent ([6b044d9](https://github.com/AlexGalhardo/vukem-video/commit/6b044d95435af0266ac812c1380ca0f4b757d770))
- **site:** Accept open and serve, listed in SDD-000, in the command mentions check ([b7e41c8](https://github.com/AlexGalhardo/vukem-video/commit/b7e41c869d75af2f3d2a65061d30bb4ff7cfb4ea))
- **ui:** Send an untouched required boolean of a palette form as its default or false ([6189c60](https://github.com/AlexGalhardo/vukem-video/commit/6189c60083c9a106a6861adf874e8110852947af))
- **core:** Say in the L_OFF_FRAME hint that x and y are the item centre from the top-left corner ([86228cc](https://github.com/AlexGalhardo/vukem-video/commit/86228cca93295e2837f6321113fe5c7feb4f083f))
- **ui:** Keep the flip of a target when pasting attributes from the menu ([5fd4c6e](https://github.com/AlexGalhardo/vukem-video/commit/5fd4c6e9b56418b85aef4bb36940654318b72f01))
- **ui:** Show the error and hint of a refused keyframe under Add at playhead ([aec0ff6](https://github.com/AlexGalhardo/vukem-video/commit/aec0ff61251d918778f4c190eca1b553925058b2))
- **ui:** Keep the hue of a neutral colour wheel until it has a tint to send ([7e0f3fc](https://github.com/AlexGalhardo/vukem-video/commit/7e0f3fc8bf920caf14e9b88f3d9454732b48d663))
- **ui:** Accept percent input in the mask fields and show their unit ([b92a6db](https://github.com/AlexGalhardo/vukem-video/commit/b92a6db104b762079fefb0251040367284db1869))
- **ui:** Coalesce slider key presses into one command and keep steps made in flight ([b8d070f](https://github.com/AlexGalhardo/vukem-video/commit/b8d070f620e273716a9bd515f8bbaad3e2388fc8))
- **api:** Tie the contract harness servers to the test process ([8d6e98a](https://github.com/AlexGalhardo/vukem-video/commit/8d6e98aaba506796beee3cc606e3be49e2cc8ff6))
- **ui:** Drop the repeated blend keys of the English catalogue ([c0f7cfd](https://github.com/AlexGalhardo/vukem-video/commit/c0f7cfd5662815a616fe5c116c9c18842dfba087))
- **ui:** Make check:i18n report a repeated key inside a catalogue object ([a3d0229](https://github.com/AlexGalhardo/vukem-video/commit/a3d022974a5adeb118e6f460607fac45641a3a70))
- **service:** Refuse a brand kit body with none of logo, colors or fonts ([377cf0a](https://github.com/AlexGalhardo/vukem-video/commit/377cf0a29a7b4f48dd91a72789e019082deb4de7))

## [0.25.0] - 2026-11-17

### Added

- **engine:** Pin per-frame colour matrices with a golden case of animated colour ([2d6e8f7](https://github.com/AlexGalhardo/vukem-video/commit/2d6e8f7d4d9602897a9dcc47e80975f7760f9e25))
- **engine:** Draw the HSL bands, colour wheels and curves of the advanced colour ([da86436](https://github.com/AlexGalhardo/vukem-video/commit/da864364041a96217798eb1c3ba5c161b5d008f7))
- **plan:** Carry the lookup table of a clip in the layer and the window ([60379f7](https://github.com/AlexGalhardo/vukem-video/commit/60379f752f3ce0ca1817e27f982d98b429747091))
- **engine:** Parse .cube lookup tables with limits and a fuzz target ([8848ab2](https://github.com/AlexGalhardo/vukem-video/commit/8848ab23e3bcefd77bb212ddf55a89963ad21152))
- **engine:** Draw .cube lookup tables in the Go compositor and the Rust renderer ([0ff4203](https://github.com/AlexGalhardo/vukem-video/commit/0ff42039d8ba316f9579c00f2b95008144ef37ce))
- **ui:** Type the grade, the lookup table and the per-frame colour matrices of a plan layer ([0331bb6](https://github.com/AlexGalhardo/vukem-video/commit/0331bb6d992fcd8faeda2d3d292f20567311646e))
- **ui:** Draw the colour matrix of each frame when the plan animates colour ([df4c547](https://github.com/AlexGalhardo/vukem-video/commit/df4c547cb59f22f69dfd2a9bfe624b40215ca08d))
- **ui:** Draw the HSL bands, colour wheels and curves of a clip in the preview ([28511d3](https://github.com/AlexGalhardo/vukem-video/commit/28511d315ca0675c9bf3e856a74904288179c2dc))
- **http:** Serve the parsed lookup table of a lut asset for the preview ([6cab0ff](https://github.com/AlexGalhardo/vukem-video/commit/6cab0ffcc223adf425541bfa4643966677f0bd83))
- **ui:** Fetch and validate the parsed lookup table of a lut asset ([e891d42](https://github.com/AlexGalhardo/vukem-video/commit/e891d42abffef151e1e3d2f396b2cece6d483f30))
- **ui:** Draw the lookup table of a clip in the preview ([03c2662](https://github.com/AlexGalhardo/vukem-video/commit/03c26626aed43a8f2644dd5753fcf1dbac3c5684))
- **ui:** Make the current time of the preview a field that seeks the playhead ([05d9f37](https://github.com/AlexGalhardo/vukem-video/commit/05d9f3734b467f213d6aad0267f570e812b5047d))
- **ui:** Save the open project as a template from the project panel ([76d6c29](https://github.com/AlexGalhardo/vukem-video/commit/76d6c29b9fa6b6871fc882d5ba8e521bd412ce04))
- **ui:** Show the content hash of an asset in Inspect for the brand kit logo ([d97e785](https://github.com/AlexGalhardo/vukem-video/commit/d97e7852804749b9d083991f67bc738a0c56d983))
- **service:** Show frames, loop duration and animated in the probe of an animated image ([16831b5](https://github.com/AlexGalhardo/vukem-video/commit/16831b54c8546296e88aa582b1b295e004b35653))
- **ui:** Show the shortcut in the tooltip of the edit tools ([5ab96a6](https://github.com/AlexGalhardo/vukem-video/commit/5ab96a6708aaab364a1a91bffb70d75dc97d180b))
- **ui:** Light the toggle buttons that are on ([e3dcead](https://github.com/AlexGalhardo/vukem-video/commit/e3dcead41c5d1d0d4f3f83837556cb35f6f0e653))
- **ui:** Show the time of the last automatic save in the top bar ([59b9568](https://github.com/AlexGalhardo/vukem-video/commit/59b95680d5135676a7748207b0bc2cf4c07ce011))
- **ui:** Group the timeline toolbar into edit tools at the left and modes and zoom at the right ([a0c5b9e](https://github.com/AlexGalhardo/vukem-video/commit/a0c5b9e23bd7f2ccad13477ef5754ee655415e36))
- **ui:** Add a reset button beside the scale, rotation and opacity fields ([fff9c7d](https://github.com/AlexGalhardo/vukem-video/commit/fff9c7de8beb718a388c82285099330bda7768aa))
- **api:** List the imported fonts with their hash in GET /fonts ([a344c14](https://github.com/AlexGalhardo/vukem-video/commit/a344c1487303c9c368ba3e66f2281f5738d20412))
- **api:** Add a beats section to media_inspect with the tempo and the first beats ([f4902d9](https://github.com/AlexGalhardo/vukem-video/commit/f4902d93b5f80ebd2931fdf5e688629fc4b2d93c))
- **api:** Add project_id to the items of the project listing ([fcbca07](https://github.com/AlexGalhardo/vukem-video/commit/fcbca0791d3f3054d2e3b796fb4d6c3082748c21))

### Fixed

- **service:** Write the .cube table as the normalized artifact the export reads ([18e7f1d](https://github.com/AlexGalhardo/vukem-video/commit/18e7f1d4d12f733b9301c908259872881d728f5d))
- **ffmpeg:** Drop the macOS linker flag that libvorbis adds and the current ld refuses ([07214ea](https://github.com/AlexGalhardo/vukem-video/commit/07214ea9c86a0459d3d789df81140243ffc58cd9))
- **ffmpeg:** Make the macOS own build reproducible with the new audio and image libraries ([1e5c21a](https://github.com/AlexGalhardo/vukem-video/commit/1e5c21aff9c731c5f6e433fa0db2fae2031eb39b))
- **http:** Make the OpenAPI document declare the statuses and the server the API really has ([4e7ba74](https://github.com/AlexGalhardo/vukem-video/commit/4e7ba74c0e54ffe489f197029e166bb49fe3c2aa))
- **api:** Give the OpenAPI the form of ids, nonce and fps and accept a numeric fps ([62032c8](https://github.com/AlexGalhardo/vukem-video/commit/62032c8114bc7cd3567f4521ca5bf35d21e5fa4f))
- **mcp:** Say the accepted values and type in a validation error and that view needs at ([83b935e](https://github.com/AlexGalhardo/vukem-video/commit/83b935e39d1e42271efe0d2f094a29b26a8810e7))
- **cli:** Show the batch file format in the help of edit --ops ([61fd54e](https://github.com/AlexGalhardo/vukem-video/commit/61fd54e66db4b797ea4ca571dbb19c87d8149c2e))
- **mcp:** Report the workspace and server in serverInfo ([4e980bf](https://github.com/AlexGalhardo/vukem-video/commit/4e980bfe168d10d582933e20a0b092063ae79ecf))
- **desktop:** Do not start the update schedule in a build without a feed ([bfee4b5](https://github.com/AlexGalhardo/vukem-video/commit/bfee4b55ebab22e37a4c4558260ed965d7279466))
- **desktop:** Pass the WebView2 browser arguments and keep one profile per workspace ([bbe6595](https://github.com/AlexGalhardo/vukem-video/commit/bbe6595290725b05e39cc065711a3863c47d6062))
- **ui:** Keep the copied attributes source between the menu entries ([58a03de](https://github.com/AlexGalhardo/vukem-video/commit/58a03deec1f90d52823d799c88a864db945ecd18))
- **ui:** Tell the person that linking needs two items instead of sending one ([b38b814](https://github.com/AlexGalhardo/vukem-video/commit/b38b81465148ea478fdb567186fe54c6c68971bc))
- **ui:** Say what a template import brought and warn when the name was already there ([5c6e844](https://github.com/AlexGalhardo/vukem-video/commit/5c6e844164b29d2026f5bb875da616d78c094a0c))
- **desktop:** Show the update dialog bodies in the language of the menu ([fda81b1](https://github.com/AlexGalhardo/vukem-video/commit/fda81b1489950d8f5bc6cdc16397a68242166456))
- **ui:** Name the open template slots in the language of the page ([217a04e](https://github.com/AlexGalhardo/vukem-video/commit/217a04eb0a919bfaa2d4dc55c7ebf1e0b7c4fa29))
- **ui:** Link an icon so the browser does not log a 404 for the favicon ([b39faef](https://github.com/AlexGalhardo/vukem-video/commit/b39faef95bcf70d45ab485cd70bb8c41292dbd97))
- **ui:** Make the caption words on the timeline named, focusable buttons that Enter edits ([315bdad](https://github.com/AlexGalhardo/vukem-video/commit/315bdad288c1cba1f76ab1b15dc2f84e26a5f9bb))
- **ui:** Add to the timeline on the track of the selected item ([71e5a9d](https://github.com/AlexGalhardo/vukem-video/commit/71e5a9d5403f70d5e385338db4b4f589346bc7c1))
- **engine:** Reach the loudness target with a peak limiter and report the shortfall ([d00ebac](https://github.com/AlexGalhardo/vukem-video/commit/d00ebacb927c665d44b0341c5d27c14196437d68))
- **core:** Scale a caption or text down to max_width when a word is wider than it ([7ba4db6](https://github.com/AlexGalhardo/vukem-video/commit/7ba4db640f041479d584df3c8c86904e037dfedb))
- **engine:** Give the ingest deadline a term for the size of the source file ([5abcf37](https://github.com/AlexGalhardo/vukem-video/commit/5abcf3794f4380ad34f563af4fc8c964e5a2fd66))
- **service:** Warn when a GIF export scales the project down and document the cap ([79c5969](https://github.com/AlexGalhardo/vukem-video/commit/79c596940eb9cba4980e4639bd911d7543d07a50))
- **ffmpeg:** Keep the macOS UUID but derive it from the binary so the build is reproducible ([4446342](https://github.com/AlexGalhardo/vukem-video/commit/44463427670fd4f84d68641b27a4c3c3e3e3a0b6))
- **ui:** Give the border of a pressed toggle 3:1 contrast against the panel ([dff510d](https://github.com/AlexGalhardo/vukem-video/commit/dff510d56b0aa7410b604f95824604da24262e7a))
- **core:** Treat project and settings as objects in the revision conflict check ([8013e91](https://github.com/AlexGalhardo/vukem-video/commit/8013e9138b88dcb4ba0ce4a557c4281d0bb95440))
- **core:** Let clip.remove_silences cut audio items ([2542e6b](https://github.com/AlexGalhardo/vukem-video/commit/2542e6b7fbcbce5539f70454ccb6d78f7a053abe))
- **core:** Refuse a shape width or height above 32768 pixels ([64f3f88](https://github.com/AlexGalhardo/vukem-video/commit/64f3f888ae090ffc22b3a070d18ca523e05664c3))
- **core:** Correct the operation help and name the resulting length of a speed overlap ([3da7fc8](https://github.com/AlexGalhardo/vukem-video/commit/3da7fc86b3930aa17efed8a592929b8e40c51f6d))
- **core:** Stop four lint rules from flagging things that are fine ([edd2de6](https://github.com/AlexGalhardo/vukem-video/commit/edd2de6141e26e3813a83b0583bbb13cc9d95940))
- **core:** Cap the loudness estimate of the lint by what the export gain can reach ([a875ae8](https://github.com/AlexGalhardo/vukem-video/commit/a875ae8ea93bef98845e493cf9523a1186524a9a))
- **core:** Give the id arguments of every command the id pattern in its schema ([50a8bfd](https://github.com/AlexGalhardo/vukem-video/commit/50a8bfd2e101500372b1e0311ecb2ac0bb346c04))
- **cli:** Let the person at the terminal install a helper while the server runs ([28d4ab0](https://github.com/AlexGalhardo/vukem-video/commit/28d4ab02423084f81371b532c2d8d2f43bc6a67d))
- **cli:** Answer an unknown command or flag in the product error format ([47b52dd](https://github.com/AlexGalhardo/vukem-video/commit/47b52dd0f7db4521905ffd6976736e655a0841a5))
- **cli:** Make open find the running server and print its launch address ([c547d18](https://github.com/AlexGalhardo/vukem-video/commit/c547d186c3f88a73848154c112f12127b6767083))
- **api:** Let any reader list the installed helpers and document the restricted routes ([966ec21](https://github.com/AlexGalhardo/vukem-video/commit/966ec21f4f30d5f70baab96f5c57eaa9212ef10e))
- **service:** Import back the bundle and template files the server exported ([a70e045](https://github.com/AlexGalhardo/vukem-video/commit/a70e045dec9355c6b701f62107b5c58318c84fae))
- **service:** Resolve the transcript of a clip created in the same batch ([0209dd4](https://github.com/AlexGalhardo/vukem-video/commit/0209dd41c88dd4201d9f4be752e8db3f27259199))
- **http:** Refuse width and height on project creation instead of dropping them ([f698f4c](https://github.com/AlexGalhardo/vukem-video/commit/f698f4cdf77bc71b4f55aea26f618452daf931dc))
- **api:** Name the commands of the surface in the hints of an error ([7d865a7](https://github.com/AlexGalhardo/vukem-video/commit/7d865a777b223e7e7a60d16fd09631a119b2b47c))
- **service:** Clear the progress message of a succeeded job ([f1ea253](https://github.com/AlexGalhardo/vukem-video/commit/f1ea2533bee71283e78174c35f58c6ca293de0b9))
- **api:** Refuse a limit or count below 1 instead of answering 200 ([4ce7903](https://github.com/AlexGalhardo/vukem-video/commit/4ce79030bd7e505dbc26b0f387f7678d475b8eb4))
- **service:** Check every asset id before a media import imports anything ([a9ca552](https://github.com/AlexGalhardo/vukem-video/commit/a9ca552c165d420c97cde81f72cffaa65c464dcb))
- **repo:** Count a release pull request from the previous release tag, not a build tag ([b50d533](https://github.com/AlexGalhardo/vukem-video/commit/b50d533fc38f2a88a0cf714d227ce8bd1885a889))

## [0.24.0] - 2026-11-17

### Added

- **engine:** Port the shape rasterizer to the Rust renderer, byte-equal with engine/raster ([11f56a0](https://github.com/AlexGalhardo/vukem-video/commit/11f56a03496f9c3472ed5cf03750e61ebfa53aed))
- **core:** Add a magnet flag to track.update that keeps a track packed ([609d66b](https://github.com/AlexGalhardo/vukem-video/commit/609d66b77470d0bc6237d8c2eff7c0ab651d5e37))
- **core:** Add a lookup table (LUT) to clip.set_color and the lut asset kind ([9d852e4](https://github.com/AlexGalhardo/vukem-video/commit/9d852e43b86b7071ea8f1cb92b563abb50f748aa))
- **core:** Add the item.freeze_frame command ([15061ab](https://github.com/AlexGalhardo/vukem-video/commit/15061abc66cd8b9b4e2d4b943e94b3fa5fc7385c))
- **repo:** Record the detection page on Chromium and Edge on Windows ([dacbbd4](https://github.com/AlexGalhardo/vukem-video/commit/dacbbd4be329a2bde5a9ace7e4ef5ae058c54932))
- **engine:** Add the approved text crates to the renderer with their Unicode-3.0 exceptions ([5331407](https://github.com/AlexGalhardo/vukem-video/commit/53314078f1ae37124b1ecc3fe270ecc47728d398))
- **engine:** Port the text layout, shaping and drawing of engine/raster to the Rust renderer ([ba5baa4](https://github.com/AlexGalhardo/vukem-video/commit/ba5baa45bb7bd5240c3c9dc55f5c2768bfbce0df))
- **ui:** Add the magnet toggle, the freeze frame action and the LUT picker ([2b92fe2](https://github.com/AlexGalhardo/vukem-video/commit/2b92fe276b43455fc0cb956152c1506c862dab19))
- **ui:** Add the voice filter select to the Audio section ([8fd2a79](https://github.com/AlexGalhardo/vukem-video/commit/8fd2a7945b01600d903523ea6ab13586a8241e41))
- **ui:** Add copy and paste attributes with Mod+Shift+C and Mod+Shift+V ([d917a5f](https://github.com/AlexGalhardo/vukem-video/commit/d917a5fd865446791573b583ef962f88e81dd304))
- **ui:** Add the markers section to the item and project inspectors ([7f1afab](https://github.com/AlexGalhardo/vukem-video/commit/7f1afabf5592c5a46f7d62e0b35bff459a80f2d0))
- **ui:** Export only the range between the in and out points ([22e3240](https://github.com/AlexGalhardo/vukem-video/commit/22e32405978363d8f2a7541817b1e60625cb7a90))
- **ui:** Export a single PNG frame at the playhead from the export dialog ([fc92d31](https://github.com/AlexGalhardo/vukem-video/commit/fc92d31448b7a2fbadbc5cf809e8e28846fb47e7))
- **core:** Carry the track magnet, lookup table and freeze frame through OTIO ([63981c5](https://github.com/AlexGalhardo/vukem-video/commit/63981c5e3378041d242487ac400fac1c04f936b8))
- **core:** Add HSL bands, colour wheels and curves to clip.set_color ([89b5a66](https://github.com/AlexGalhardo/vukem-video/commit/89b5a66d829794bd95b6a4aef1151d5692c4be0f))
- **core:** Animate the colour properties of a clip with keyframe.set ([b48221a](https://github.com/AlexGalhardo/vukem-video/commit/b48221ac86d1caf870993547b561ea39b37efd92))
- **core:** Add the spin, shrink and grow transitions and carry every kind through OTIO ([33c9ae3](https://github.com/AlexGalhardo/vukem-video/commit/33c9ae38822b50939b310c6395e7c0104c0d9712))
- **core:** Add caption.import_subtitles and the core/subtitle SRT and WebVTT parser ([7c39d17](https://github.com/AlexGalhardo/vukem-video/commit/7c39d17054385815106c8a329069f5bd7e9de68a))
- **engine:** Move the shared math of the text rasterizer so the renderer can link it ([2d7670e](https://github.com/AlexGalhardo/vukem-video/commit/2d7670e92f5b297b0ce021c4d4fab408aaf76680))
- **engine:** Add the text exports to the renderer module and the wazero host ([6a5fce7](https://github.com/AlexGalhardo/vukem-video/commit/6a5fce79df951b46825ec1f24a55f119141253d4))
- **engine:** Make the Rust text rasterizer the reference of every bitmap ([f9340e8](https://github.com/AlexGalhardo/vukem-video/commit/f9340e87fc8c514869f8838e67118ebef4d22b6f))
- **engine:** Switch every caller to the Rust rasterizer and remove engine/raster ([9516ad5](https://github.com/AlexGalhardo/vukem-video/commit/9516ad53a057df7ace6185af24b1f8c61b1c86e5))
- **engine:** Compile the renderer module at service start, not at the first text ([959fc74](https://github.com/AlexGalhardo/vukem-video/commit/959fc74c258632d2a113165ad1c044afb9e9bae2))
- **ui:** Add HSL, colour wheels and curves to the Colour section ([cdc3126](https://github.com/AlexGalhardo/vukem-video/commit/cdc31262bc7e6da172e5d14d412cd29362621f38))
- **ui:** Add colour keyframes to the Colour section ([2417dc3](https://github.com/AlexGalhardo/vukem-video/commit/2417dc38a5373bbda39ef46c1a3b23d06c6c4835))
- **ui:** Add the Import subtitles file picker for SRT and WebVTT ([31c091e](https://github.com/AlexGalhardo/vukem-video/commit/31c091e674824172c4a567a1b61fa53378ec3e5f))
- **service:** Allow the lut asset kind in the assets table ([14170a0](https://github.com/AlexGalhardo/vukem-video/commit/14170a0a0514a3e0e160dd08cf7575d6b5d5d8bf))
- **service:** Ingest a .cube file as an asset of kind lut ([46a4dfd](https://github.com/AlexGalhardo/vukem-video/commit/46a4dfd28b02c369bb66e28cdb83f0a2f20e45ef))

### Fixed

- **auth:** Treat only an admin browser session as privileged for doctor and tools ([23bc005](https://github.com/AlexGalhardo/vukem-video/commit/23bc005f5624c098037eb7c5d6e095cdcf13b783))
- **http:** Key the frames-in-flight cap by the credential, not the client label ([693d289](https://github.com/AlexGalhardo/vukem-video/commit/693d289cd5e7094af5ca850eb95402e40d4ddb33))
- **cli:** Announce the OpenH264 install from its own manifest ([e1ae786](https://github.com/AlexGalhardo/vukem-video/commit/e1ae786e493ac80537b8a18e577ed9f4104cef37))
- **service:** Split the .cube checker and list a lut asset in the project view ([daad534](https://github.com/AlexGalhardo/vukem-video/commit/daad5345dc4fcbcef7886bdd5da50b71f3fd0a0a))

### Security

- **security:** Add the independent 1.0 security review ([d3e9318](https://github.com/AlexGalhardo/vukem-video/commit/d3e9318a9ee53641c5336170259abef7e573991c))

## [0.23.0] - 2026-11-16

### Added

- **engine:** Draw the pixel effects and the whip, dip and wipe in the compositor ([44ed5d2](https://github.com/AlexGalhardo/vukem-video/commit/44ed5d275d5b3e82dd49b5f710add10b2b642e2a))
- **renderer:** Port the pixel effects, whip, dip and wipe, byte-equal with the Go compositor ([895b46f](https://github.com/AlexGalhardo/vukem-video/commit/895b46faee11197b7dc8370c49df9f58a17a74c9))
- **renderer:** Draw the wipe coverage on the GPU path ([65e45d9](https://github.com/AlexGalhardo/vukem-video/commit/65e45d95e884feda65c92d7e30889514e1b134f3))
- **export:** Play a speed curve and reverse in the exported audio, and test the video frames ([ce436c1](https://github.com/AlexGalhardo/vukem-video/commit/ce436c1bf02361246d302677238a36b60df74d8b))
- **ui:** Decode the video proxy with WebCodecs in a worker ([8f7bd08](https://github.com/AlexGalhardo/vukem-video/commit/8f7bd085637c7c301a500c8e9908d7c1749b86d2))
- **ui:** Seek frame exact and keep a bounded decode queue ahead of the playhead ([c509fb5](https://github.com/AlexGalhardo/vukem-video/commit/c509fb56768808ce0bc29a6f2db7f1609eb02c13))
- **ui:** Fall back to the video element per asset and force either path in tests ([544b1d8](https://github.com/AlexGalhardo/vukem-video/commit/544b1d838838c00c197f23c9546777d3884c0c55))
- **ui:** Bound the open VideoFrames and check it in the leak and soak specs ([0c43483](https://github.com/AlexGalhardo/vukem-video/commit/0c434834d18eacff695f341269636a0049d8114c))
- **core:** Add item_id to marker.add, marker.set and marker.remove ([adbad74](https://github.com/AlexGalhardo/vukem-video/commit/adbad7455b8cf398387a1a090996afd30a803bba))
- **core:** Add the item.paste_attributes command ([d4e8582](https://github.com/AlexGalhardo/vukem-video/commit/d4e858240e739b0ffb0c239ae7cabe825bb00b34))
- **core:** Add voice presets to item.set_audio ([921b0f0](https://github.com/AlexGalhardo/vukem-video/commit/921b0f07b3e31423e950be2b15536e14f66687dd))
- **engine:** Extend the hostile corpus with the BETA inputs and snapshot the build listings ([e382fcc](https://github.com/AlexGalhardo/vukem-video/commit/e382fccc1fcf271b593b9f63f990d4b72066c78d))

### Fixed

- **release:** Generate the doctor pins from the own FFmpeg block when FFMPEG_SOURCE is own ([98f10de](https://github.com/AlexGalhardo/vukem-video/commit/98f10def314dd97268dc960a695ce93491c113b1))
- **servicetest:** Take the bench revision from the result and start with a real move ([2a47576](https://github.com/AlexGalhardo/vukem-video/commit/2a4757679e2fd5b9dac235951e6a6ab85162991e))

### Security

- **security:** Test the verdict of the no-egress strace scan on fixtures ([f589b0e](https://github.com/AlexGalhardo/vukem-video/commit/f589b0e9fc065364197489aa479ccf207a517492))
- **security:** Print the failing command and refuse root in the no-egress run ([65690bc](https://github.com/AlexGalhardo/vukem-video/commit/65690bc5986129b89b1da2dbd020aa6d4ed576f2))
- **security:** Add worker-src 'self' to the CSP for the same-origin preview workers ([9f5bc05](https://github.com/AlexGalhardo/vukem-video/commit/9f5bc05dd3779422840cfd2fb9980bf159cbe471))
- **security:** Allow the sample admin password of the quick start and the timing script ([8268fda](https://github.com/AlexGalhardo/vukem-video/commit/8268fda7625af837a36d9c4b7697042b66922ee8))

## [0.22.0] - 2026-11-16

### Added

- **ui:** Add builders for the speed curve, reverse, effects, link and extract audio ([66f1427](https://github.com/AlexGalhardo/vukem-video/commit/66f1427ae3c0b4f12f541cbedd9ac3b76e9acd44))
- **ui:** Add the speed curve editor and the Reverse toggle to the Speed section ([fb3f7bb](https://github.com/AlexGalhardo/vukem-video/commit/fb3f7bb491b9258832aa24e71c42a78fd6ed0ba9))
- **preview:** Play a speed curve and a reversed item from the plan ([747dba4](https://github.com/AlexGalhardo/vukem-video/commit/747dba4d915c2c03b1038565d16cad6e780ef326))
- **ui:** Link, unlink and extract audio from the item menu and the keyboard ([8436485](https://github.com/AlexGalhardo/vukem-video/commit/8436485654aa0d501dabf798d53c3d0d5ad6e9bc))
- **ui:** Add the Effects section for clip.set_effects ([e86583c](https://github.com/AlexGalhardo/vukem-video/commit/e86583cd57a14a6faffc3342e0eddf0a7ca97d9a))
- **ui:** Add the colour and the direction to the transition controls ([a9f7ce4](https://github.com/AlexGalhardo/vukem-video/commit/a9f7ce456cf40dc00ce216abb9d0fefc2c0b955a))
- **preview:** Draw the pixel effects, the whip blur, the dip and the wipe ([559a761](https://github.com/AlexGalhardo/vukem-video/commit/559a7617d68739b41e755909fcfc54f92eb65849))
- **ui:** Add Shift+B speed curve, Shift+M marker jumps and Alt+wheel scroll keys ([9a8c2d8](https://github.com/AlexGalhardo/vukem-video/commit/9a8c2d897f157abb0a2307188537a9dd5eb6adb3))

### Fixed

- **export:** Verify the nominal frame rate so a VideoToolbox export is not refused ([0a32ad6](https://github.com/AlexGalhardo/vukem-video/commit/0a32ad60fbf38a2d06c510e24d4d766aa8332337))
- **preview:** Play on the wall clock when the audio context cannot run ([bd8a33f](https://github.com/AlexGalhardo/vukem-video/commit/bd8a33fe1316ad42a46d98c18b2effd781c1f5fe))
- **service:** List the created ids of an edit in operation order ([75b30b8](https://github.com/AlexGalhardo/vukem-video/commit/75b30b85c984ee8bb6eb09fd94b48fc8f36bdcef))
- **ffmpeg:** Mask the AVX-512 code of FFmpeg on x86-64 so media bytes do not depend on the CPU ([ceb8d2f](https://github.com/AlexGalhardo/vukem-video/commit/ceb8d2fb3c4e1e8caee1e6f93cfd9937c00a2ade))
- **core:** Refuse a slide whose extended neighbour would overlap another item ([048d0ce](https://github.com/AlexGalhardo/vukem-video/commit/048d0ce45373a7a2285be6d65dee722996ff605e))
- **ui:** Read the speed fractions through a units helper ([bc10ef5](https://github.com/AlexGalhardo/vukem-video/commit/bc10ef5cfa660cd1d730a888f3aced4abc1d1c88))
- **ui:** Keep the curve point arithmetic in the builders ([fd6f1aa](https://github.com/AlexGalhardo/vukem-video/commit/fd6f1aab5965095e08ecab30ca1663d01e754513))

### Security

- **security:** Run the TestSecurity_ tests on the three operating systems ([e8ab2f4](https://github.com/AlexGalhardo/vukem-video/commit/e8ab2f420f584a7d90229ad6b3d8ede487d3b8fd))
- **security:** Fail a security test that skips without its platform or a documented condition ([e24d19f](https://github.com/AlexGalhardo/vukem-video/commit/e24d19febfd8647c7321ec5f1ee2d3207f363366))

## [0.21.0] - 2026-11-16

### Added

- **web:** Draw masks in the preview and add the Mask inspector section ([20e1e01](https://github.com/AlexGalhardo/vukem-video/commit/20e1e015e4874b87da0b7ab2e838d9ad7463a3ab))
- **desktop:** Sign published_at and expires in the update feed and refuse a stale one ([6850e8b](https://github.com/AlexGalhardo/vukem-video/commit/6850e8bd424c9c655b18f51f7375b59d4980486a))
- **ui:** Add an in-editor find bar on Mod+F ([fa29465](https://github.com/AlexGalhardo/vukem-video/commit/fa29465d714861d4e397846ccbfa1cc1ca627342))
- **ui:** Add the blade and select tools, range keys, keyframe panel key and Mod-to-ignore-guides ([11af877](https://github.com/AlexGalhardo/vukem-video/commit/11af8775f0c09b25b00e3b9825d5e39f9023cf11))
- **workspace:** Let a network-mode owner prove itself with a per-run secret before the key is sent ([1a6cc72](https://github.com/AlexGalhardo/vukem-video/commit/1a6cc72b7ff1d16ea8355d4f1c3c09da66147157))
- **engine:** Draw masks in the compositor ([3ae652e](https://github.com/AlexGalhardo/vukem-video/commit/3ae652e5856573c97fad9505f487d3f988672dde))
- **engine:** Draw masks in the Rust renderer, byte-equal with the Go compositor ([1c61472](https://github.com/AlexGalhardo/vukem-video/commit/1c61472914dd62b13b73602027573740a1da5db8))
- **engine:** Draw masks on the GPU path ([5297cdb](https://github.com/AlexGalhardo/vukem-video/commit/5297cdb55cd9c8e6fd7e3d330783f9ff699e7b00))
- **ingest:** Keep the alpha of a source in a WebM proxy and PNG thumbnails ([adaec4b](https://github.com/AlexGalhardo/vukem-video/commit/adaec4bce62d9ace4f44e7adc200622f2ab35f36))
- **cli:** Show progress, honour --timeout and refuse --no-wait in process ([aa899a8](https://github.com/AlexGalhardo/vukem-video/commit/aa899a89a9a5b140c3a6b0d47b4ed37dedfac4ed))
- **release:** Carry the release notes and the launcher artifacts in the update feed ([0be4b60](https://github.com/AlexGalhardo/vukem-video/commit/0be4b60abd3e7eca932d19e94528b3bac92a5b28))
- **desktop:** Add the launcher that keeps the editor up to date ([48a87b9](https://github.com/AlexGalhardo/vukem-video/commit/48a87b96908ba0fec6a3a02359bcc50f8e34964f))
- **desktop:** Install updates on macOS and Linux ([c40d29d](https://github.com/AlexGalhardo/vukem-video/commit/c40d29db18f50602fb6976256c7eddd1adca9528))
- **core:** Add clip.set_flip and plan flip fields ([f3b6a13](https://github.com/AlexGalhardo/vukem-video/commit/f3b6a131cd0ed84dfe409a930c4a6b57e2ef4acb))
- **core:** Add item.set_enabled and the disabled item field ([8ffaece](https://github.com/AlexGalhardo/vukem-video/commit/8ffaece3f6e26050ffe7c60480f8e46d341b5000))
- **core:** Add white balance and hue to clip.set_color ([e8e3c15](https://github.com/AlexGalhardo/vukem-video/commit/e8e3c15dd211d7e9d6ac665bfa9cc1a3f306cb14))
- **core:** Add noise reduction strength to item.set_audio ([ab2bdce](https://github.com/AlexGalhardo/vukem-video/commit/ab2bdce45ff66ccd0497ce61ddd29115410f348e))
- **ui:** Add flip, enable, white balance, hue and noise reduction controls ([15c8274](https://github.com/AlexGalhardo/vukem-video/commit/15c8274ba8b3e575799fb9ae3eff0405680a69fc))
- **site:** Use the viewfinder mark for the favicon, the docs navbar and the README ([43d9a93](https://github.com/AlexGalhardo/vukem-video/commit/43d9a93104b7a4132f6f6f51ca88f31d8e862737))
- **engine:** Draw flip_x and flip_y in the compositor ([7a1bc02](https://github.com/AlexGalhardo/vukem-video/commit/7a1bc02bcb71fdf25c85cc6893e099406973fbf1))
- **engine:** Draw flip_x and flip_y in the Rust renderer, byte-equal with the Go compositor ([22ba834](https://github.com/AlexGalhardo/vukem-video/commit/22ba834a3b813864e64893cf585392b9666fc26a))
- **engine:** Reduce the noise of a source with denoise through afftdn in the export ([579cc06](https://github.com/AlexGalhardo/vukem-video/commit/579cc06081ad945eecbe16056ea21853c07a2057))
- **site:** Add a Scalar-style API reference generated from the OpenAPI document ([39c9a75](https://github.com/AlexGalhardo/vukem-video/commit/39c9a75b9a37e01a2f541107e3f37c66329dd2c5))
- **site:** Add an MCP reference with one page per tool and per edit operation ([0707a6b](https://github.com/AlexGalhardo/vukem-video/commit/0707a6bc3ca1192fda0018768a220e898f2feb44))
- **site:** Make the desktop sidebar an accordion like the mobile drawer ([6baf944](https://github.com/AlexGalhardo/vukem-video/commit/6baf944da8961a53448901ad915769c4523e011c))
- **core:** Add a speed curve to clip.set_speed ([8c5c1ea](https://github.com/AlexGalhardo/vukem-video/commit/8c5c1ea3143e3d38452a449c5f42b7e53d3bd9c2))
- **core:** Add reverse to clip.set_speed ([6682d12](https://github.com/AlexGalhardo/vukem-video/commit/6682d126d5e3d4aeecceca648db65bc984abd116))
- **core:** Add item.link, item.unlink and item.extract_audio ([6258824](https://github.com/AlexGalhardo/vukem-video/commit/6258824cd2228291350813f74d9a8fa45469e6df))
- **core:** Add clip.set_effects and the zoom, whip, dip and wipe transitions ([7f1ff09](https://github.com/AlexGalhardo/vukem-video/commit/7f1ff096b7a033e434d50f8716a30d12f85f07d0))
- **core:** Carry captions, shapes, transitions and text effects in the OTIO vukem metadata ([b72e2db](https://github.com/AlexGalhardo/vukem-video/commit/b72e2db1a03ccd6fa93c792043b577e8c2a75587))

### Fixed

- **service:** Attribute imported bundle history to imported actors and close it to undo ([aa2032a](https://github.com/AlexGalhardo/vukem-video/commit/aa2032a144e04d3899de8f7c0468a814db22991e))
- **mcp:** Make the local actor the client name plus a session id ([01159c5](https://github.com/AlexGalhardo/vukem-video/commit/01159c592c72287302c7a7ef51a40cff4be7e321))
- **desktop:** Recover a pending update whose watchdog died ([fcdad07](https://github.com/AlexGalhardo/vukem-video/commit/fcdad0771e0d6acecc80c14e8ddfc6d2d98badc7))
- **desktop:** Prefer the launcher artifacts of the feed on macOS and Linux ([06c6572](https://github.com/AlexGalhardo/vukem-video/commit/06c657211d168876c071044d2ba2e88d58fb07c6))
- **desktop:** Reconcile the feed freshness and the launcher entries after integration ([9192b33](https://github.com/AlexGalhardo/vukem-video/commit/9192b3345e5696c04bf61e11ee0f49449805d487))
- **ci:** Recognise the Unicode-3.0 licence text in the release audit ([dace9b7](https://github.com/AlexGalhardo/vukem-video/commit/dace9b7532f695c4df31cbb6eef478c3be1fcdec))
- **cli:** Document user:create and user:passwd as taking an email ([38f12c6](https://github.com/AlexGalhardo/vukem-video/commit/38f12c6e1f28c80397cb6893c927a5baf0b0d88e))
- **site:** Run the latest image on the landing and match the navbar width ([3217838](https://github.com/AlexGalhardo/vukem-video/commit/321783816b94bd9eac712145a68195b26dfd471b))
- **desktop:** Start the detached process with a context ([e3d8a2b](https://github.com/AlexGalhardo/vukem-video/commit/e3d8a2b70c4475c7ce76fc04b46bb7e39be622c6))
- **site:** Keep test:scripts free of dependencies ([70d64ef](https://github.com/AlexGalhardo/vukem-video/commit/70d64ef2900d3e5644c4cbadc5463e1ce77776b8))
- **ci:** Encode the upgrade test clip with the LGPL mpeg4 encoder ([da1b4b4](https://github.com/AlexGalhardo/vukem-video/commit/da1b4b4e0a4c8532b9c023be500a8f26cf62907e))
- **ci:** List the release versions without mapfile for the bash 3.2 of macOS ([b610913](https://github.com/AlexGalhardo/vukem-video/commit/b610913d53342710880ea08f22d876a9e003081f))
- **auth:** Give a browser session its privileges only for a launch code with the launcher secret ([053737f](https://github.com/AlexGalhardo/vukem-video/commit/053737fd47184749e90431dcde32d2a87aef6f14))
- **auth:** State the real limit of plain HTTP in a container and warn when the bind is not loopback ([2d917f6](https://github.com/AlexGalhardo/vukem-video/commit/2d917f6a309a23dfceef39a387679024a7f18237))
- **site:** Drop the licence line from the landing ([2c1595f](https://github.com/AlexGalhardo/vukem-video/commit/2c1595ff3c54e765b919ba2f74ad9e2f075b3b5c))
- **site:** Add the trailing slash to pages named after a dotted operation ([f7b7f0d](https://github.com/AlexGalhardo/vukem-video/commit/f7b7f0dbb0251416f4fc1e980a74b421b7249db1))
- **web:** Load the translations in every build of the app, not only with the API ([764528b](https://github.com/AlexGalhardo/vukem-video/commit/764528b2d17bcc9ae5445e91089ffb8c49c20542))
- **desktop:** Report a missing server binary as a spawn failure on macOS too ([9ed30ca](https://github.com/AlexGalhardo/vukem-video/commit/9ed30ca44c253c4a122ca6d9e9f3970bb1a9828a))
- **cli:** Treat the context deadline as the in-process timeout without waiting for the timer ([a182e59](https://github.com/AlexGalhardo/vukem-video/commit/a182e591057c2865df0c4c98977a54c1ba0b5b7c))
- **service:** Retry the atomic rename that Windows refuses while a reader holds the target ([0d0d808](https://github.com/AlexGalhardo/vukem-video/commit/0d0d80863f304f6138ecae149edbbc697903d6ee))

### Security

- **security:** Allow the usage lines of the CLI help test in the leak scan ([c31b916](https://github.com/AlexGalhardo/vukem-video/commit/c31b916d56088f7dcfa3f8ca36e7a9a7c9cb8646))
- **security:** Allow the type-name placeholders of the generated API reference ([035dfce](https://github.com/AlexGalhardo/vukem-video/commit/035dfcefe2c55a1ee8233ef630a5ae18a8d100b0))

## [0.20.0] - 2026-11-16

### Added

- **web:** Draw the chroma key in the preview and add its inspector section ([973ae76](https://github.com/AlexGalhardo/vukem-video/commit/973ae769483258a470707f300ec8722e99426f5f))
- **web:** Add markers, J/K/L and in/out points to the editor keys ([0f5e8ab](https://github.com/AlexGalhardo/vukem-video/commit/0f5e8abf57e1514ad8c399598bde6be2ca5fdf9a))
- **desktop:** Print the page with Ctrl+P in the window ([3bcfe29](https://github.com/AlexGalhardo/vukem-video/commit/3bcfe298cb93772b321bbc0e57eebb4975bcd9d0))
- **web:** Explain in the library why a browser imports by path ([9041803](https://github.com/AlexGalhardo/vukem-video/commit/90418034c15c6717618e842d94af741b1f66950e))
- **cli:** Add --out to export, bundle:export and template:export ([04b7aed](https://github.com/AlexGalhardo/vukem-video/commit/04b7aedc0d356b5d62755879b781a61f7ba89054))
- **core:** Add roll, slide and slip edits ([e9397ad](https://github.com/AlexGalhardo/vukem-video/commit/e9397ad85d9b053d23ddb155e240340a8348ae5d))
- **core:** Add settings.reframe to re-target a project to another aspect ([587083c](https://github.com/AlexGalhardo/vukem-video/commit/587083c939b9a3f618d0c04195a44ec7afac700a))
- **core:** Crossfade two neighbours of one track through a second track ([cfce71d](https://github.com/AlexGalhardo/vukem-video/commit/cfce71db5b9720884c1e744a04378b9706393612))
- **ui:** Show a finished export in its folder from the desktop window ([1946345](https://github.com/AlexGalhardo/vukem-video/commit/1946345a83ea9a92c80c1c2211239d495ecffebb))
- **ui:** Pick the template file with the native dialog in the desktop window ([5a7772d](https://github.com/AlexGalhardo/vukem-video/commit/5a7772d2e7830698462060921ea1b17dddeeb57a))
- **engine:** Draw the frame plan on the GPU with wgpu ([df147ed](https://github.com/AlexGalhardo/vukem-video/commit/df147ed6618abac1badfe3551070bf907b53fec1))
- **engine:** Hold the GPU path to the CPU path by SSIM and geometry ([d1ce87f](https://github.com/AlexGalhardo/vukem-video/commit/d1ce87f0cb85385f5a6d065d3dad315212f21746))
- **service:** Warn at the start of a GIF export whose typical size is over 25 MB ([b267007](https://github.com/AlexGalhardo/vukem-video/commit/b2670075b6cea3422526c7912931f8d1f83e84b4))
- **service:** Let a transcribe job pick an installed whisper model ([61a8d83](https://github.com/AlexGalhardo/vukem-video/commit/61a8d83dc50f8fa7a7302f0cc343d6e20ffdc161))
- **service:** Export one frame as a png and accept the whole view token range ([6b336af](https://github.com/AlexGalhardo/vukem-video/commit/6b336af51df622a6850e5d286725e70af3538fc5))
- **ui:** Crossfade two neighbours of one track through another visual track ([e37604c](https://github.com/AlexGalhardo/vukem-video/commit/e37604cea8875fe615cf612bddc6256ffc87dc9f))
- **ui:** Change the project aspect with settings.reframe ([0d70b1a](https://github.com/AlexGalhardo/vukem-video/commit/0d70b1aaaad7b7690fbc39edc22b25f66d6d896b))
- **ui:** Roll, slide and slip from the keyboard ([5f88cfd](https://github.com/AlexGalhardo/vukem-video/commit/5f88cfd487db8da14a95ad03f4f19c88f439d033))
- **cli:** Add --model to transcribe ([9c9116e](https://github.com/AlexGalhardo/vukem-video/commit/9c9116e014b75dc1e5eec1924a27934aed5c8d08))
- **cli:** Add --at to export for a png frame ([2ad8256](https://github.com/AlexGalhardo/vukem-video/commit/2ad8256d6d3b177dccdc5886cc1acc958b8652ca))
- **ui:** Choose the whisper model in the install dialog ([33e2b2c](https://github.com/AlexGalhardo/vukem-video/commit/33e2b2c418e6f25db6809b3f0443d23bd56cd3e6))
- **engine:** Run the GPU path in a child process fed through pipes ([4e17577](https://github.com/AlexGalhardo/vukem-video/commit/4e17577c6e74d8043a028815a2ecb3e1802c1322))
- **engine:** Draw the export on the GPU host when there is an adapter and the config allows ([13fd041](https://github.com/AlexGalhardo/vukem-video/commit/13fd0414145451614af6bb7503bfb8e39ec3bf20))
- **service:** Locate vukem-render and pass it to the GPU export ([568e960](https://github.com/AlexGalhardo/vukem-video/commit/568e9602dd4ae341c41c7cb8584de0aaa2b7f791))
- **core:** Add blend modes to visual items (clip.set_blend and plan.Layer.Blend) ([b323715](https://github.com/AlexGalhardo/vukem-video/commit/b32371560789c7af55d79cd1b6967203b9b106d2))
- **otio:** Keep speed, keyframes, slots, texts and chroma key in metadata.vukem ([5efaebe](https://github.com/AlexGalhardo/vukem-video/commit/5efaebe3dcdb1c0881f94098ec08fdd248a5d98a))
- **otio:** Carry the blend mode of clips and text items in metadata.vukem ([2e3793f](https://github.com/AlexGalhardo/vukem-video/commit/2e3793f192adf083ca2387cee2806a68780e524c))
- **otio:** Round-trip the document markers through the timeline metadata and the stack markers ([e0a9bd6](https://github.com/AlexGalhardo/vukem-video/commit/e0a9bd627b9d2e73edfe8f4fd61c30bc23dfd7bc))
- **web:** Draw the blend modes in the preview and add the inspector Blend section ([c5e797f](https://github.com/AlexGalhardo/vukem-video/commit/c5e797ff04350f7d5b2db5ec952bbe3e1e8250b9))
- **engine:** Draw blend modes in the compositor ([8dc3b9a](https://github.com/AlexGalhardo/vukem-video/commit/8dc3b9ab14fad201f7cec5e65e9ebd2e4820e907))
- **engine:** Draw blend modes in the Rust renderer, byte-equal with the Go compositor ([53f1ce1](https://github.com/AlexGalhardo/vukem-video/commit/53f1ce1c2f3fe6a5ae643a608c112ad056a5e27a))
- **engine:** Draw blend modes on the GPU path ([2d39d96](https://github.com/AlexGalhardo/vukem-video/commit/2d39d967fd4de451de6185cd37d8d08b504cf89d))
- **core:** Add masks to visual items (clip.set_mask and plan.Layer.Mask) ([7b79a12](https://github.com/AlexGalhardo/vukem-video/commit/7b79a12ee9646fcd1c32b875f3e11f20d79f6d7c))

### Fixed

- **web:** Name Choose files and drops in the first-run library tip of the desktop window ([da40377](https://github.com/AlexGalhardo/vukem-video/commit/da403772cbc9bbb35cdc511dc61a5c37dab33d22))
- **web:** Show the own actor as you instead of a raw id ([0870115](https://github.com/AlexGalhardo/vukem-video/commit/08701155469f09a11ad07de99f0bb3bb53a9fb6b))
- **cli:** Read the --out file only from the own server when the connection is remote ([67f5936](https://github.com/AlexGalhardo/vukem-video/commit/67f59361fcb0ef39126634cafb46bf0c20023729))
- **ui:** Paste copied items at the playhead and make cut then paste work ([8b3730f](https://github.com/AlexGalhardo/vukem-video/commit/8b3730fe78983167fffb976c84fa89f1ee5b8928))
- **service:** Omit dimensions and an empty duration from the job text of a bundle export ([ba1c3a8](https://github.com/AlexGalhardo/vukem-video/commit/ba1c3a8c50b1e86849dd911c778b6212441c5cf6))
- **service:** Record the installed whisper model as the workspace default ([742ebba](https://github.com/AlexGalhardo/vukem-video/commit/742ebbac350c7b8af7e608abf106bdfc131115de))
- **service:** Show the vukem-render path in the doctor only to a privileged caller ([33725bf](https://github.com/AlexGalhardo/vukem-video/commit/33725bf30aa51c502ea344312214af95b867d23f))
- **desktop:** End the server with the shell on Linux and macOS and show the startup error ([79aeb93](https://github.com/AlexGalhardo/vukem-video/commit/79aeb934d4715427e59e075da3c6db7010c7d4bd))
- **desktop:** Keep the runtime.json of a live owner when the workspace is locked ([1cb0d6b](https://github.com/AlexGalhardo/vukem-video/commit/1cb0d6b5f64ef27964e9a6c35a0e149c232eb816))

## [0.19.0] - 2026-11-16

### Added

- **engine:** Write a chunk index next to the video proxy ([dfafcb8](https://github.com/AlexGalhardo/vukem-video/commit/dfafcb842a9a145b6e22376b40694bb759005d65))
- **engine:** Serve the chunk index with the proxy ([138ad1d](https://github.com/AlexGalhardo/vukem-video/commit/138ad1df3d92bc8d878ab5893141cbbbe184c3f1))
- **ui:** CapCut-compatible keyboard shortcuts ([00793f4](https://github.com/AlexGalhardo/vukem-video/commit/00793f4903c266188e16711e84a9617f00a6ff5a))
- **export:** Export a range of the timeline ([acc42ca](https://github.com/AlexGalhardo/vukem-video/commit/acc42ca4f1aea01b21c55d88ccee7bc3500a178a))
- **core:** Let caption.set_words take text and redistribute the timings ([80a5f66](https://github.com/AlexGalhardo/vukem-video/commit/80a5f6633cb37114300e8403fe2d245a60b30208))
- **core:** Caption from transcript on an audio item ([dd13b16](https://github.com/AlexGalhardo/vukem-video/commit/dd13b16c2195f273f97da30910d832aef59156fd))
- **core:** Add chroma key to clips (clip.set_chroma_key and plan.Layer.ChromaKey) ([cf1fbd6](https://github.com/AlexGalhardo/vukem-video/commit/cf1fbd6dac0b0fb34924104754c88bfa8ff6f839))
- **otio:** Keep clip and track properties in vukem metadata and say what an import did ([ebd5352](https://github.com/AlexGalhardo/vukem-video/commit/ebd5352d77c26ec97fd86ad48bfb1a8aaebbac4d))
- **mcp:** Add the font_import tool and put the args schema and item shapes in command_help ([da055a3](https://github.com/AlexGalhardo/vukem-video/commit/da055a38bc40b63228756971d427de10a9d089c5))
- **cli:** Restore export --start and --end now that the engine exports a range ([26c958d](https://github.com/AlexGalhardo/vukem-video/commit/26c958d4c4d35c9ef370caadab402b1e0767ff96))
- **core:** Add timeline markers (marker.add, marker.set, marker.remove) ([19bbe6f](https://github.com/AlexGalhardo/vukem-video/commit/19bbe6fdb0f4c466819ef6a7420b5583efcb65fe))
- **core:** Lock, solo and volume on tracks, and ripple on item.trim ([5f95c37](https://github.com/AlexGalhardo/vukem-video/commit/5f95c37a7f81a70bf775d376a3d5bbc84f37010b))
- **engine:** Port the compositor to the Rust CPU path, byte-equal with Go ([ad763ba](https://github.com/AlexGalhardo/vukem-video/commit/ad763baf58ef460264f25a50b37387f1e261ad1d))
- **engine:** Add the Rust plan decoder, the renderer state and the Go parity harness ([93feaad](https://github.com/AlexGalhardo/vukem-video/commit/93feaad069e3fbcbc005c2d1893b36cf04c341ca))
- **engine:** Embed the Rust core module and run it from Go with wazero ([f05bbda](https://github.com/AlexGalhardo/vukem-video/commit/f05bbda4a97c9a041ca577e15b4d8950e63f69ab))
- **engine:** Choose the Go compositor or the Rust renderer for the export ([7567b69](https://github.com/AlexGalhardo/vukem-video/commit/7567b699db06c27486b0edc6c1cc4f9fc137892a))
- **engine:** Draw chroma key in the compositor ([f3fe414](https://github.com/AlexGalhardo/vukem-video/commit/f3fe4141ed85bb5888196f17ee62e3450008f69c))
- **engine:** Draw chroma key in the Rust renderer, byte-equal with the Go compositor ([165b6de](https://github.com/AlexGalhardo/vukem-video/commit/165b6de4a1d58e25269eb51876c99ddb5a2315e2))
- **command:** Report a batch that changes nothing so it commits no revision ([06df7ab](https://github.com/AlexGalhardo/vukem-video/commit/06df7abc591e5ca39db68514616fa078f89384fa))
- **export:** Name the downloaded file after the project and the date ([8938a04](https://github.com/AlexGalhardo/vukem-video/commit/8938a04b19fc9526e20c05fcb7fb58486bfa40b7))
- **frame:** Compose a lossless full-resolution still of a frame ([bb32048](https://github.com/AlexGalhardo/vukem-video/commit/bb320480d2338ef3e9cb7de700c78ff42954d9b2))
- **ingest:** Keep the alpha plane of a VP9 video in WebM ([93a3248](https://github.com/AlexGalhardo/vukem-video/commit/93a3248ebb20c658d8addfdaacc8c6651364424e))
- **export:** Estimate the size of a GIF before it is written ([5753dfe](https://github.com/AlexGalhardo/vukem-video/commit/5753dfe3627ff6fea0fc3f416631734c25babff7))

### Fixed

- **export:** Cap a GIF at 480 pixels wide and 15 fps by default ([b98fcb0](https://github.com/AlexGalhardo/vukem-video/commit/b98fcb006740fc28f46498eb426a5506ba5317b9))
- **service:** Keep projects and assets with active jobs out of a trash emptying ([5c717ec](https://github.com/AlexGalhardo/vukem-video/commit/5c717ec37836c226ae3db02d60e3ee675135a47f))
- **remote:** Report a timed-out write as an unknown outcome, not a lock ([1a50feb](https://github.com/AlexGalhardo/vukem-video/commit/1a50feb26339ba5e27e4cfbcb223ad1d9059bb5c))
- **web:** Keep the interface language per browser instead of a shared cookie ([c8c2785](https://github.com/AlexGalhardo/vukem-video/commit/c8c278536fa863be9cce5141550f30e9d196637f))
- **pathguard:** Tell the caller to copy a file out of the workspace ([92e1e5e](https://github.com/AlexGalhardo/vukem-video/commit/92e1e5eb8cd0b6cadacf33c006c5c8ecaff3b52d))
- **service:** Read the summary out of the probe artifact the ingest writes ([e3a0618](https://github.com/AlexGalhardo/vukem-video/commit/e3a0618473dec271a714825c74a397ada3e5c487))
- **service:** Refuse an export range at validation instead of queueing a job that fails ([a609b94](https://github.com/AlexGalhardo/vukem-video/commit/a609b94603f7698f9b6cd3cc2e1502b26ec1514b))
- **service:** Refuse a sheet of an empty timeline and name until in the times error ([37ae2f6](https://github.com/AlexGalhardo/vukem-video/commit/37ae2f60bd5e4364d2898f7ef0308dc5055c82fe))
- **mcp:** List every public command in the op enum of the edit tool ([487d9b0](https://github.com/AlexGalhardo/vukem-video/commit/487d9b0db9bfe17df9eb90d3fa151841e14cce1c))
- **cli:** Say what is wrong with a brand kit or a template fill and show the expected shape ([a68fb74](https://github.com/AlexGalhardo/vukem-video/commit/a68fb74fa3e29008250b5f1b4f5643c5f7c02924))
- **service:** Show where an export is in the job text and drop the stale progress message ([4648306](https://github.com/AlexGalhardo/vukem-video/commit/4648306890bdd912c55ca3a99c56e71502170807))
- **ffmpeg:** Tell the person that SVG is not supported instead of calling it a fake media file ([ca97d82](https://github.com/AlexGalhardo/vukem-video/commit/ca97d82d6c3fdd144a0bd8e6d659dad53dbc306e))
- **cli:** Give E_WORKSPACE_LOCKED a hint that says what to close ([7c46ee8](https://github.com/AlexGalhardo/vukem-video/commit/7c46ee8409ff170d24bb99933957d61e799b3eb4))
- **cmd:** Let the stdio MCP server take a free port when the default one is busy ([cbbda72](https://github.com/AlexGalhardo/vukem-video/commit/cbbda7290521ba1377b7fee5a32e38c07eb21874))
- **service:** Read a history actor filter with no kind as a local actor and say what an asset id looks like ([82b04a3](https://github.com/AlexGalhardo/vukem-video/commit/82b04a3528a7ac9ca33c3ce67d288b121f424895))
- **service:** Word the dry run of template save and new as what would happen ([db6cd83](https://github.com/AlexGalhardo/vukem-video/commit/db6cd8326ba773876a0c7b288d8a86d589bb911c))
- **cli:** Report one version from every command and stamp it in every build ([6b43422](https://github.com/AlexGalhardo/vukem-video/commit/6b4342284119fbd00927e5474334c7cb9cc365d6))
- **service:** One project name rule for desktop and network, 431 as E_PAYLOAD_TOO_LARGE ([d8f811e](https://github.com/AlexGalhardo/vukem-video/commit/d8f811e2a297e621dd795b52ac990f60dd19616f))
- **ui:** Limit the inspector number fields to the registry ranges and keep a whole-axis crop from being sent ([dc83704](https://github.com/AlexGalhardo/vukem-video/commit/dc837043b38f4af331d4b41bca179fb8213ba92a))
- **lint:** Stop three false positives of L_DURATION_LIMIT, L_OFF_FRAME and L_TRANSITION_EMPTY ([331e819](https://github.com/AlexGalhardo/vukem-video/commit/331e819bdbec3647e777e2ba3a624b3094e9b0b7))
- **core:** Refuse audio settings on images and say why audio analysis has nothing to analyse ([7fe00d9](https://github.com/AlexGalhardo/vukem-video/commit/7fe00d96d84d228e514371e97f1fad54fdf6a3c6))
- **service:** Show the import roots of a refused import to administrators only ([689efdf](https://github.com/AlexGalhardo/vukem-video/commit/689efdf0fa85ec76515747826747b27b9fc29331))
- **view:** Remove the asset_id member that could only answer not available ([9245200](https://github.com/AlexGalhardo/vukem-video/commit/9245200d2c5363c13b4b41046ffe62e25392da4b))
- **core:** Put a new audio track below the visual ones by default ([4d55f81](https://github.com/AlexGalhardo/vukem-video/commit/4d55f812c44a704719ef2d57a18b96626bcd4247))
- **service:** Keep the starter template frozen while audio tracks default to the bottom ([6492ba4](https://github.com/AlexGalhardo/vukem-video/commit/6492ba46b44c714ee85c85c5fb3ef52285e0fd41))
- **cli:** Answer help from the product command table and document serve ([8c822fc](https://github.com/AlexGalhardo/vukem-video/commit/8c822fcfe240f46c2035004323a98266b48538d6))
- **ui:** Retry the stored-language reload until Inertia holds its first page ([aac860a](https://github.com/AlexGalhardo/vukem-video/commit/aac860a6a3943c44d69d28a71afd7c7b146771e7))
- **ui:** Add the palette title of clip.set_chroma_key in both languages ([6d3e65d](https://github.com/AlexGalhardo/vukem-video/commit/6d3e65d14e078e85d8fd78abfdbcd83fb0bd50a2))
- **core:** Merge caption.set_style into the existing override and document highlight ([44d987d](https://github.com/AlexGalhardo/vukem-video/commit/44d987d6e948617c16c1dddfa83c53284b4f044a))
- **core:** State the identifier rule in the not an id message ([d59e5cc](https://github.com/AlexGalhardo/vukem-video/commit/d59e5cc0183e2e140e25a98f1ac85ef58621c2f4))
- **ui:** Add the palette titles of the marker commands in both languages ([1119054](https://github.com/AlexGalhardo/vukem-video/commit/1119054667ef98a7843bc4c9ae171881c9266e2c))
- **cli:** Expand ~ and file:// paths and name the missing path, without OS-language read errors ([3d8fc01](https://github.com/AlexGalhardo/vukem-video/commit/3d8fc0131760fa58149a2ae61777788f0962e094))
- **service:** Commit no revision for an edit that changes nothing ([2421926](https://github.com/AlexGalhardo/vukem-video/commit/24219269ae2747c0a48040947bdfd20271ef5451))

### Security

- **security:** Fail when product code calls service.EditInternal ([779a108](https://github.com/AlexGalhardo/vukem-video/commit/779a108b381ef180eb557a4d2747e62b108351dc))

## [0.18.0] - 2026-11-16

### Added

- **ui:** Go to the previous and next cut with the Up and Down keys ([8775a6d](https://github.com/AlexGalhardo/vukem-video/commit/8775a6d0031ac79de85decc3922c9de536ef3f58))
- **engine:** Keep the alpha channel of video overlays ([abf90cb](https://github.com/AlexGalhardo/vukem-video/commit/abf90cb9551c9e4a37971f4a1d3835abc6ea8e8c))
- **ui:** Pick the PixiJS renderer, WebGPU when an adapter exists and WebGL otherwise ([cb4dab3](https://github.com/AlexGalhardo/vukem-video/commit/cb4dab32be8aabe1cf359f52b20dbf65c745aee3))

### Removed

- Keep media_import a successful MCP result when every file failed ([288489e](https://github.com/AlexGalhardo/vukem-video/commit/288489edd1f9a6624a68d17df2647f1be1b91c7c))

### Fixed

- **ci:** Build FFmpeg under a fixed path so both rounds match ([16f0815](https://github.com/AlexGalhardo/vukem-video/commit/16f0815f08a33da1919a63d365a49c5b5117eac3))
- **ui:** Seek preview audio to the source time of the current frame ([b170223](https://github.com/AlexGalhardo/vukem-video/commit/b170223a0d71df1fb3789908561f34d4db37f9eb))
- **ffmpeg:** Give internal run directories unique names ([ca29c9f](https://github.com/AlexGalhardo/vukem-video/commit/ca29c9fface2b2d6bb8ea4fda965bcff7ea89bcd))
- **service:** Make every job reach a final state ([5d0c769](https://github.com/AlexGalhardo/vukem-video/commit/5d0c7699c8f970aebe070e70cff3b0b9796901c0))
- **ui:** Let the export dialog start another export after one finished ([2062aaa](https://github.com/AlexGalhardo/vukem-video/commit/2062aaae941bdb739e514605ca8dfce3fd8e6b44))
- **ui:** Show the load-failed banner when the first load fails on the network ([a5b967a](https://github.com/AlexGalhardo/vukem-video/commit/a5b967a880dfca85fc5e6668144893897c4f4ec5))
- **ffmpeg:** Reclaim the leftover run directory of a retried job ([d0ff11b](https://github.com/AlexGalhardo/vukem-video/commit/d0ff11b00616ac9542584e3b87b7801f5b7e71a1))
- **events:** Drop the export path from job events in network mode ([d96198e](https://github.com/AlexGalhardo/vukem-video/commit/d96198e92bb722e298793c2b2015c05536b99d82))
- **ui:** Give the logo link an accessible name when appName is empty ([4063920](https://github.com/AlexGalhardo/vukem-video/commit/4063920560262696fc119d49eb0475d826980739))
- **ui:** Send an expired session from any editor call to the login ([1a0e6dd](https://github.com/AlexGalhardo/vukem-video/commit/1a0e6ddb1b93c94a7edec8a863e92059be05fb7c))
- **ui:** Give an empty timeline track a cell so its row is valid ARIA ([a60a0d8](https://github.com/AlexGalhardo/vukem-video/commit/a60a0d880d04075e3ec21a1c3679550793d4e9f7))
- **ui:** Clear the alert of a projects form when another form is submitted ([ea4d99f](https://github.com/AlexGalhardo/vukem-video/commit/ea4d99fa67466e1c1ddf8c8edb525d1ece62ae70))
- **ui:** Start the palette button label with its visible text ([845b46b](https://github.com/AlexGalhardo/vukem-video/commit/845b46b2327322b7a631eb4c3079668481f986ba))
- **http:** Accept the loopback aliases of APP_URL in the container rule ([2240206](https://github.com/AlexGalhardo/vukem-video/commit/22402065bbf481ada7b0700c9cc5fe53a8d22104))
- **core:** Bound extreme numeric arguments instead of panicking on +Inf ([6a9abf3](https://github.com/AlexGalhardo/vukem-video/commit/6a9abf3575db168ce876c53d0049dc9dd2dcb933))
- **ui:** Put the edit toolbar of the editor inside a landmark ([7ede144](https://github.com/AlexGalhardo/vukem-video/commit/7ede1440d672ad9144da9fef80db591210c85c53))
- **ui:** Name unnamed tracks V1/A1 and unnamed ducking items by their media ([468daf4](https://github.com/AlexGalhardo/vukem-video/commit/468daf4de83781e291c51741be6c1be7ffa9ea87))
- **ui:** Show the limits of a refused number and hide the argument path in the inspector ([475c203](https://github.com/AlexGalhardo/vukem-video/commit/475c203a882895f30582c1982b39abe697e79f78))
- **ui:** Mark a template slot with a default name when its toggle is switched on ([94df832](https://github.com/AlexGalhardo/vukem-video/commit/94df83247eedb165f4f9be71ee9d539e063b5381))
- **service:** Remove invisible characters from untrusted text ([5ec7cf5](https://github.com/AlexGalhardo/vukem-video/commit/5ec7cf54aafa251b6b582c8d0d1a13c7be3182fc))
- **service:** Key the heavy limit by credential ([72ebaec](https://github.com/AlexGalhardo/vukem-video/commit/72ebaec05a28fc83e1d0b4d2b567837b3d0bc80b))
- **desktop:** Give the Choose files dialog media filters ([590c8c8](https://github.com/AlexGalhardo/vukem-video/commit/590c8c887696d2aacdf510b3fe2a399f8243b184))
- **ui:** Show every time in the editor through one formatter and the time display preference ([486b7db](https://github.com/AlexGalhardo/vukem-video/commit/486b7dbee2cbe822b85de4da3fae19748d7bfc38))
- **desktop:** Reuse the loopback port of the last run so browser preferences persist ([307db08](https://github.com/AlexGalhardo/vukem-video/commit/307db08c003041dc1fd0b706ee46a70d3eb97bd7))
- **desktop:** Bind F5, page zoom and a QA-only F12 in the window ([5d8b0c2](https://github.com/AlexGalhardo/vukem-video/commit/5d8b0c2c2176d62190d35a068923b9dace12b541))
- **export:** Stop composing frames once the encoder has exited ([bc83ed2](https://github.com/AlexGalhardo/vukem-video/commit/bc83ed2c469da09338068dbaaa05a67bba9ef27e))
- **service:** Count assets whose ingest failed in media import failures ([986dcf0](https://github.com/AlexGalhardo/vukem-video/commit/986dcf00da9cbb9bf3b71e80e7866d47c03cd848))
- **cli:** Send rooted paths as typed when talking to a remote server ([dc3d617](https://github.com/AlexGalhardo/vukem-video/commit/dc3d617995f023167d50ebfb151e1e5cda9e0050))
- **desktop:** Tie the server child to the shell with a job object ([2df8854](https://github.com/AlexGalhardo/vukem-video/commit/2df8854a0ec79c3c3546ac9ae65c5d6a46fd02f1))
- **owner:** Transcribe with the installed whisper model when none is configured ([87fa654](https://github.com/AlexGalhardo/vukem-video/commit/87fa654a843d185e2970d05f8e9693db31981b0e))
- **desktop:** Show the native menu and dialogs in the system language ([d9b75e3](https://github.com/AlexGalhardo/vukem-video/commit/d9b75e368f28323d1ae37e1addfb289870ba8b24))
- **ui:** Apply the next free slot only to the operation that failed ([1223d1d](https://github.com/AlexGalhardo/vukem-video/commit/1223d1d8cbd6b4fcf4e524d783742ce73e6ca06e))
- **export:** Restart a single-frame decoder for every new source frame ([6774bf7](https://github.com/AlexGalhardo/vukem-video/commit/6774bf7b4cdcb8e331b9837812c4b34f50c857ec))
- **preview:** Never ask for the frame past the end of the timeline ([a00ac7b](https://github.com/AlexGalhardo/vukem-video/commit/a00ac7b0ed80f5733dbf1c4b2565bed5532b3ad9))
- **library:** Keep typing during an import and accept several paths ([dff6b15](https://github.com/AlexGalhardo/vukem-video/commit/dff6b15eb191fc791c09a5423b84a1551196e156))
- **projects:** Create a project through the JSON API, without the 409 ([c505f2d](https://github.com/AlexGalhardo/vukem-video/commit/c505f2dc9c0126791989a9274ee885e84c7ed827))
- **export:** Refuse wav and aac exports of a project with no sound ([9906744](https://github.com/AlexGalhardo/vukem-video/commit/9906744126a110300877c4ab479fb52563583c63))
- **library:** Show an icon for audio assets instead of asking for a thumbnail ([4ee5756](https://github.com/AlexGalhardo/vukem-video/commit/4ee5756c4e454088b34a0136d2d576ac8500ce9c))
- **mcp:** Report media_import as an error when every file failed ([6ba63ec](https://github.com/AlexGalhardo/vukem-video/commit/6ba63ecebe0814077b759b3ae79adc44954cc7ef))
- **core:** Let text, shape and caption pick a free visual track ([6a38034](https://github.com/AlexGalhardo/vukem-video/commit/6a38034f18e22d351ec51e34eb1772b89304b3de))
- **docker:** Install libgomp1 so the whisper helper starts in the image ([a2165e3](https://github.com/AlexGalhardo/vukem-video/commit/a2165e33751775b4b4da9df23bccbdaa276cc563))
- **core:** Make beat markers readable in the diff text and in project detail full ([7595e20](https://github.com/AlexGalhardo/vukem-video/commit/7595e204ebb5676f63c33c8e228c1ed4198c87f9))
- **http:** Include the project state in the project ETag ([6c0f9c2](https://github.com/AlexGalhardo/vukem-video/commit/6c0f9c26c8f841f0d1b9599633f94e1c9ff1f762))
- **http:** Answer 200 for a dry run given in the query ([8e92843](https://github.com/AlexGalhardo/vukem-video/commit/8e9284331ca163c9932409290465dcb655e1d164))
- **http:** Look operation names up exactly, as MCP does ([b0ff2cc](https://github.com/AlexGalhardo/vukem-video/commit/b0ff2ccbee6eec3321e73b4091e55b4e200ffbb1))
- **cli:** Resume the live stream after resync.required ([b2e7da0](https://github.com/AlexGalhardo/vukem-video/commit/b2e7da0b3d48013a293352b56953f8fb653f2525))
- **web:** Share the signed-in user and the app name in network mode ([5ffd28e](https://github.com/AlexGalhardo/vukem-video/commit/5ffd28ef8977032eb7c6ea54ffc618c9f044f0d3))
- **web:** Answer pages that do not exist with the error page ([ed19ec9](https://github.com/AlexGalhardo/vukem-video/commit/ed19ec9841399ed8abf96f35e593e2233f181b2d))
- **core:** Add an overlay image at its natural size instead of filling the frame ([23cd0c8](https://github.com/AlexGalhardo/vukem-video/commit/23cd0c835fb924cd0cf191c9d2217cb11e828df9))
- **notices:** Parse SPDX expressions with their parentheses ([03a7b36](https://github.com/AlexGalhardo/vukem-video/commit/03a7b364299fc12d2e7bee891af0c880897c5654))
- **core:** Resolve a missing font weight to the nearest the family has ([a46aaf4](https://github.com/AlexGalhardo/vukem-video/commit/a46aaf46ed072b42ba945a04728d5f8782c0de09))
- **core:** Place a duplicate at the next free position when start is omitted ([3ba164b](https://github.com/AlexGalhardo/vukem-video/commit/3ba164b073669068a536932c818690d47978c082))
- **core:** Rescale caption words by their two ends on set_fps ([4a60202](https://github.com/AlexGalhardo/vukem-video/commit/4a602028f0d6d5090fa26e7d08ec052b5184408a))
- **service:** Show the revision a dry run would commit as ([cad4db0](https://github.com/AlexGalhardo/vukem-video/commit/cad4db0137422b4e86c0746a7a34a1d71f4c7b33))
- **service:** Answer E_ASSET_NOT_READY for an edit on an asset still ingesting ([e873380](https://github.com/AlexGalhardo/vukem-video/commit/e8733800b1562d89b86401cd4588b0facf1cbdf7))
- **lint:** Do not report L_OFF_FRAME for a box that covers the frame ([5b4f282](https://github.com/AlexGalhardo/vukem-video/commit/5b4f282ead155cc4b7e22807477ec0f2fbea86db))
- **ui:** Keep the preview live frame and timecode in step with a paused seek ([14081d0](https://github.com/AlexGalhardo/vukem-video/commit/14081d0c83434701e9091b181b07c76afdf03d2d))
- **store:** Refuse a stored file that is not a regular file ([b39556b](https://github.com/AlexGalhardo/vukem-video/commit/b39556b3eb8739dddc9bb9ea68009248a5c053ad))

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

[Unreleased]: https://github.com/AlexGalhardo/stuarty/compare/v0.41.0...HEAD
[0.41.0]: https://github.com/AlexGalhardo/stuarty/compare/v0.40.0...v0.41.0
[0.40.0]: https://github.com/AlexGalhardo/stuarty/compare/v0.39.0...v0.40.0
[0.39.0]: https://github.com/AlexGalhardo/stuarty/compare/v0.38.0...v0.39.0
[0.38.0]: https://github.com/AlexGalhardo/stuarty/compare/v0.37.0...v0.38.0
[0.37.0]: https://github.com/AlexGalhardo/stuarty/compare/v0.36.0...v0.37.0
[0.36.0]: https://github.com/AlexGalhardo/stuarty/compare/v0.35.0...v0.36.0
[0.35.0]: https://github.com/AlexGalhardo/stuarty/compare/v0.34.0...v0.35.0
[0.34.0]: https://github.com/AlexGalhardo/stuarty/compare/v0.33.0...v0.34.0
[0.33.0]: https://github.com/AlexGalhardo/stuarty/compare/v0.32.0...v0.33.0
[0.32.0]: https://github.com/AlexGalhardo/stuarty/compare/v0.31.0...v0.32.0
[0.31.0]: https://github.com/AlexGalhardo/stuarty/compare/v0.30.0...v0.31.0
[0.30.0]: https://github.com/AlexGalhardo/stuarty/compare/v0.29.0...v0.30.0
[0.29.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.28.0...v0.29.0
[0.28.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.27.0...v0.28.0
[0.27.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.26.0...v0.27.0
[0.26.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.25.0...v0.26.0
[0.25.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.24.0...v0.25.0
[0.24.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.23.0...v0.24.0
[0.23.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.22.0...v0.23.0
[0.22.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.21.0...v0.22.0
[0.21.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.20.0...v0.21.0
[0.20.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.19.0...v0.20.0
[0.19.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.18.0...v0.19.0
[0.18.0]: https://github.com/AlexGalhardo/vukem-video/compare/v0.17.0...v0.18.0
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
