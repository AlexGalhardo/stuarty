# The build machine of the own FFmpeg (epic 46, ADR-0011): the Linux and the Windows (mingw-w64) targets are built
# in this image, so two builds on one host see the same compiler and the same tools. The workflow
# .github/workflows/ffmpeg-build.yml builds in the same Debian image, with the same packages.
#
#   docker build -f .config/ffmpeg-build/builder.Dockerfile -t stuarty-ffbuild:1 .config/ffmpeg-build
#   docker run --rm -v "$PWD:/src:ro" -v "$OUT:/out" stuarty-ffbuild:1 \
#       bash -c 'cp -r /src /repo && cd /repo && bash scripts/ffmpeg-build.sh linux amd64 /out'
FROM golang:1.27.4@sha256:5e8afd493a7124a8586e134d9b532731161d4b9becf9dee5a5956b107426f93d AS go

FROM debian:13.7-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2
RUN apt-get update \
	&& apt-get install -y --no-install-recommends build-essential ca-certificates curl git binutils perl pkg-config nasm \
		autoconf automake libtool cmake zip xz-utils patch mingw-w64 g++-mingw-w64-x86-64-posix \
	&& rm -rf /var/lib/apt/lists/* \
	&& git config --global --add safe.directory '*'
COPY --from=go /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:$PATH GOTOOLCHAIN=local
WORKDIR /repo
