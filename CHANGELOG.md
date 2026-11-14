# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/AlexGalhardo/vukem-video/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/AlexGalhardo/vukem-video/releases/tag/v0.1.0
