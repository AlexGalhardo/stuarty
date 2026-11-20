Stuarty is the product that was called Vukem Video up to `v0.29.0`; the version numbers continue. Nothing has to be done to keep your projects.

## What happens to your files

At its first start Stuarty looks for `~/.stuarty`. When that folder does not exist and `~/.vukem-video` does, it copies the database to `backups/adopt-<UTC timestamp>/`, renames the folder to `~/.stuarty` (or, when the two are on different volumes, copies it, checks every file and leaves a `MOVED-TO-STUARTY.txt` note in the old folder), writes `adopted.json` in the new one and prints one line that says so. The old folder is never deleted. A server or desktop application of Vukem Video that is still running stops the adoption: close it first. When both folders exist, `~/.stuarty` is used and one line names the old one.

## The command

`vukem-video` is now `stuarty`. A package of Vukem Video is replaced by the `stuarty` package, and the installers keep a `vukem-video` command that runs `stuarty`, so an agent configuration or a script that calls it keeps working.

## The container

The image is `ghcr.io/alexgalhardo/stuarty` (and `alexgalhardo/stuarty` on Docker Hub). A Docker volume keeps the name you gave it: keep `-v vukem-video:/data` to keep your data, or copy the volume once if you prefer the new name. The image itself sets `STUARTY_MODE`, `STUARTY_HOME`, `STUARTY_FFMPEG` and `STUARTY_FFPROBE`, so a `-e VUKEM_VIDEO_MODE=...` (or `_HOME`, `_FFMPEG`, `_FFPROBE`) given to the new image is ignored with a warning: pass those four under their new names.

## The variables

Every `VUKEM_VIDEO_*` variable is still read, until `1.0.0`, when the matching `STUARTY_*` variable is not set. Each use prints one warning; set the new name to silence it. The new name wins when both are set. The pairs:
