# Bundled fonts

The six open families of D-022, embedded in the renderer module with `include_bytes!` (`renderer/stuarty-renderer-text/src/fonts.rs`). The rasterizer reads no other font: not the system fonts, not a download (ADR-0012, D-016).

Every file comes from the Google Fonts repository at commit `cc39c4c2c304bc368c9b7c311ea2bcc6013299fe` (folder `ofl/<family>/`), which distributes the upstream projects named below under the SIL Open Font License 1.1. The licence of each family is in `licenses/fonts/<Family>-OFL.txt`; the copyright line of each text names the upstream authors. The METADATA of each family in that commit says `license: "OFL"`.

| Family | File | Weights shipped | SHA-256 of the file | Upstream | Licence |
| --- | --- | --- | --- | --- | --- |
| Inter (default) | `Inter-Variable.ttf` | 100 to 900, every step of 100 (variable `wght`, `opsz` at its default) | `29160a80ff49ddcab2c97711247e08b1fab27a484a329ce8b813d820dc559031` | <https://github.com/rsms/inter> | SIL OFL 1.1, `licenses/fonts/Inter-OFL.txt` |
| Montserrat | `Montserrat-Variable.ttf` | 100 to 900 (variable `wght`) | `0f7b311b2f3279e4eef9b2f968bcdbab6e28f4daeb1f049f4f278a902bcd82f7` | <https://github.com/JulietaUla/Montserrat> | SIL OFL 1.1, `licenses/fonts/Montserrat-OFL.txt` |
| Anton | `Anton-Regular.ttf` | 400 | `a4ba3a92350ebb031da0cb47630ac49eb265082ca1bc0450442f4a83ab947cab` | <https://github.com/googlefonts/AntonFont> | SIL OFL 1.1, `licenses/fonts/Anton-OFL.txt` |
| Bebas Neue | `BebasNeue-Regular.ttf` | 400 | `08e4623805102d819f58601e46e345648846075e363b2ceb23313c2d1c83ec73` | <https://github.com/dharmatype/Bebas-Neue> | SIL OFL 1.1, `licenses/fonts/BebasNeue-OFL.txt` |
| Playfair Display | `PlayfairDisplay-Variable.ttf` | 400 to 900 (variable `wght`) | `c40f2293766a503bc70cce9e512ef844a4ccb7cbcde792fe2ea31d191917d8d6` | <https://github.com/aaronbell/Playfair-Display> | SIL OFL 1.1, `licenses/fonts/PlayfairDisplay-OFL.txt` |
| JetBrains Mono | `JetBrainsMono-Variable.ttf` | 100 to 800 (variable `wght`) | `48715a42ec242c21e9f02692891e147d022299a52e48d5e413e1a942193ffeda` | <https://github.com/JetBrains/JetBrainsMono> | SIL OFL 1.1, `licenses/fonts/JetBrainsMono-OFL.txt` |

A weight outside these ranges, or one that is not a multiple of 100, is `E_FONT_UNKNOWN` with the weights that exist (`FontSet::lookup`). The weights are the ranges of the `wght` axis of each file; the `fonts` tests of `renderer/stuarty-renderer-text` and `TestBundledFamilies` of `engine/rasterize` fail when this table is wrong.

## Coverage

The files cover Latin, Latin Extended and Vietnamese. Inter also covers Greek and Cyrillic; Montserrat and Playfair Display cover Cyrillic. None of the six covers Arabic, Hebrew, Devanagari, CJK or emoji: a character the font lacks is drawn with the `.notdef` glyph and there is no fallback to another font (SDD-004 section 5.1 item 9). ADR-0012 records the measurement.

## Adding or updating a font

1. Take the file and its `OFL.txt` from the upstream release or from the Google Fonts folder, and check the licence in the copyright text.
2. Put the file here, the licence in `licenses/fonts/`, add a row above with the SHA-256 (`sha256sum`), and add the family to `FAMILIES` in `renderer/stuarty-renderer-text/src/fonts.rs` and to `bundledFamilies` in `engine/rasterize/fonts_test.go`.
3. Bump `BitmapVersion` in `core/plan` when the change alters the bytes of a bitmap, regenerate the vectors (`VUKEM_WRITE_RASTER_VECTORS=1 cargo test -p stuarty-renderer-text` in `renderer/`), rebuild the module (`bash scripts/build-renderer.sh embed`) and refresh the goldens (`go test ./engine/rasterize -run Golden -update`).
4. A new file needs the author's approval of the licence and of the size it adds to the binary.
