# The Stuarty image: the stuarty binary (server, CLI and the UI assets)
# and a pinned LGPL FFmpeg. Published to ghcr.io/alexgalhardo/stuarty and
# alexgalhardo/stuarty (Docker Hub) by release.yml; see
# docs/sdds/009-releases-and-ci-cd.md section 11. Build from the repository root:
#
#   docker build -f .config/docker/stuarty.Dockerfile -t stuarty .
#
# Run it on your machine only (the published port is bound to loopback):
#
#   docker run --rm -p 127.0.0.1:7878:7878 -v stuarty:/data stuarty
#
# No web server runs here: the Go binary serves HTTP. The product bundles and
# links no GPL component: .config/ffmpeg.env pins the FFmpeg, the BtbN lgpl build or the own one (ADR-0011);
# the packages of the Debian base are listed with their licences.
ARG BUN_IMAGE=oven/bun:1.4.4-alpine@sha256:4cf82f5686b196504fe119db3fcd9185b191f82ece82c40680e597228ec6070a
ARG GO_IMAGE=golang:1.27.4@sha256:5e8afd493a7124a8586e134d9b532731161d4b9becf9dee5a5956b107426f93d
ARG RUNTIME_IMAGE=debian:13.7-slim@sha256:a72b4433de1cf7d6bae0c4cee74d0547f58601a643b3375f070c51600d63b6b2

# Web assets. Platform independent, so built once on the build machine.
FROM --platform=$BUILDPLATFORM ${BUN_IMAGE} AS assets
WORKDIR /src/web
COPY web/package.json web/bun.lock ./
# The lockfile is a required build input: the image installs exactly the
# versions the repository tracks.
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build \
	&& find resources/public/build -type f -name "*.js" | grep -q .

# The static binary, cross compiled for the target platform.
FROM --platform=$BUILDPLATFORM ${GO_IMAGE} AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=devel
WORKDIR /src
ENV CGO_ENABLED=0 GOWORK=off
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=assets /src/web/resources/public/build web/resources/public/build
# The version of the binary is the one source of every command that prints it
# (`version`, `doctor`, the agent docs): agentdocs.Version, set here with -X from
# VERSION. A release, or image.yml with a local dummy tag, passes VERSION and the
# build fails when `stuarty version` prints anything else; VERSION=devel keeps
# the version written in the source and skips the check.
RUN stamp=""; \
	if [ "$VERSION" != devel ]; then stamp="-X github.com/AlexGalhardo/stuarty/web/app/agentdocs.Version=$VERSION"; fi \
	&& GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -buildvcs=false -trimpath -ldflags="-s -w $stamp" -o /out/stuarty ./web/cmd/app \
	&& if [ "$VERSION" != devel ] && [ "$TARGETARCH" = "$(go env GOHOSTARCH)" ]; then \
		got="$(/out/stuarty version)"; \
		[ "$got" = "stuarty $VERSION" ] || { echo "version stamp: got [$got], want [stuarty $VERSION]" >&2; exit 1; }; \
	fi

# The pinned FFmpeg, downloaded by exact URL and verified by SHA-256. FFMPEG_SOURCE
# chooses the pin: btbn (the BtbN lgpl build, through ALPHA) or own (the build of
# .github/workflows/ffmpeg-build.yml, D-017); empty takes the value of .config/ffmpeg.env.
# docker build --build-arg FFMPEG_SOURCE=own ...
FROM --platform=$BUILDPLATFORM ${RUNTIME_IMAGE} AS ffmpeg
ARG TARGETARCH
ARG FFMPEG_SOURCE=
COPY .config/ffmpeg.env /tmp/ffmpeg.env
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl xz-utils \
	&& rm -rf /var/lib/apt/lists/* \
	&& want="$FFMPEG_SOURCE" \
	&& . /tmp/ffmpeg.env \
	&& source="${want:-$FFMPEG_SOURCE}" \
	&& case "$source-$TARGETARCH" in \
		btbn-amd64) asset="$FFMPEG_ASSET_LINUX_AMD64"; sum="$FFMPEG_SHA256_LINUX_AMD64" ;; \
		btbn-arm64) asset="$FFMPEG_ASSET_LINUX_ARM64"; sum="$FFMPEG_SHA256_LINUX_ARM64" ;; \
		own-amd64) asset="$FFMPEG_OWN_ASSET_LINUX_AMD64"; sum="$FFMPEG_OWN_SHA256_LINUX_AMD64" ;; \
		own-arm64) asset="$FFMPEG_OWN_ASSET_LINUX_ARM64"; sum="$FFMPEG_OWN_SHA256_LINUX_ARM64" ;; \
		*) echo "unsupported FFMPEG_SOURCE-architecture: $source-$TARGETARCH" >&2; exit 1 ;; \
	esac \
	&& if [ "$source" = own ]; then \
		[ -n "$FFMPEG_OWN_TAG" ] && [ -n "$asset" ] || { echo "no own FFmpeg build is pinned in .config/ffmpeg.env" >&2; exit 1; }; \
		url="https://github.com/AlexGalhardo/stuarty/releases/download/${FFMPEG_OWN_TAG}/${asset}"; \
	else url="https://github.com/BtbN/FFmpeg-Builds/releases/download/${FFMPEG_RELEASE_TAG}/${asset}"; fi \
	&& curl -fsSL -o /tmp/ffmpeg.tar.xz "$url" \
	&& echo "${sum}  /tmp/ffmpeg.tar.xz" | sha256sum -c - \
	&& mkdir -p /opt/ffmpeg \
	&& tar -xJf /tmp/ffmpeg.tar.xz -C /opt/ffmpeg --strip-components=1 \
	&& test -x /opt/ffmpeg/bin/ffmpeg && test -x /opt/ffmpeg/bin/ffprobe && test -s /opt/ffmpeg/LICENSE.txt \
	&& echo "$source" > /opt/ffmpeg/pin-kind.txt \
	&& if [ "$source" = own ]; then test -s /opt/ffmpeg/SOURCE.txt; \
	else printf "%s\n%s\n%s\n" \
		"FFmpeg ${FFMPEG_VERSION} (LGPL build) from https://github.com/BtbN/FFmpeg-Builds/releases/tag/${FFMPEG_RELEASE_TAG}" \
		"Build scripts: https://github.com/BtbN/FFmpeg-Builds/tree/${FFMPEG_BUILDS_COMMIT}" \
		"Corresponding FFmpeg source: ${FFMPEG_SOURCE_URL}" > /opt/ffmpeg/SOURCE.txt; fi

FROM ${RUNTIME_IMAGE}
ARG VERSION=devel
ARG REVISION=unknown

LABEL org.opencontainers.image.title="stuarty" \
	org.opencontainers.image.description="Stuarty: a minimal video editor operated by AI agents and by a human UI" \
	org.opencontainers.image.source="https://github.com/AlexGalhardo/stuarty" \
	org.opencontainers.image.url="https://github.com/AlexGalhardo/stuarty" \
	org.opencontainers.image.documentation="https://github.com/AlexGalhardo/stuarty#readme" \
	org.opencontainers.image.licenses="MIT" \
	org.opencontainers.image.version="${VERSION}" \
	org.opencontainers.image.revision="${REVISION}"

# curl is the health check client; tzdata and ca-certificates are the runtime
# data the binary needs; util-linux provides prlimit, which the child process
# limits of P2-RND-005 use; libgomp1 is the OpenMP runtime the whisper.cpp helper
# (installed later by tools:install) links against (GCC runtime library exception,
# so it adds no copyleft to the image). Debian slim keeps /usr/share/doc/<package>/copyright.
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl libgomp1 tzdata util-linux \
	&& rm -rf /var/lib/apt/lists/* \
	&& useradd --system --no-create-home --uid 10001 --user-group app \
	&& mkdir -p /data /usr/share/doc/stuarty/ffmpeg \
	&& chown app:app /data \
	&& dpkg-query -W > /usr/share/doc/stuarty/base-image-packages.txt

COPY --from=ffmpeg /opt/ffmpeg/bin/ffmpeg /opt/ffmpeg/bin/ffprobe /opt/ffmpeg/bin/
COPY --from=ffmpeg /opt/ffmpeg/LICENSE.txt /usr/share/doc/stuarty/ffmpeg/LICENSE.txt
COPY --from=ffmpeg /opt/ffmpeg/SOURCE.txt /usr/share/doc/stuarty/ffmpeg/SOURCE.txt
COPY --from=ffmpeg /opt/ffmpeg/pin-kind.txt /tmp/ffmpeg-pin-kind.txt
# The licence obligations of the LGPL build (ADR-0011 item 2): the licence text,
# the configure line and where the corresponding source is (SOURCE.txt, above).
# The configure line is read from the binary that runs in this stage: it must be
# LGPL, and the own build (D-017) must not have OpenH264 compiled in.
RUN /opt/ffmpeg/bin/ffmpeg -version | sed -n "s/^configuration: //p" > /usr/share/doc/stuarty/ffmpeg/configure.txt \
	&& test -s /usr/share/doc/stuarty/ffmpeg/configure.txt \
	&& ! grep -q -e "--enable-gpl" -e "--enable-nonfree" /usr/share/doc/stuarty/ffmpeg/configure.txt \
	&& if [ "$(cat /tmp/ffmpeg-pin-kind.txt)" = own ]; then \
		! grep -q -e "--enable-libopenh264" /usr/share/doc/stuarty/ffmpeg/configure.txt; \
	fi \
	&& rm /tmp/ffmpeg-pin-kind.txt

COPY LICENSE.md THIRD_PARTY_NOTICES.md /usr/share/doc/stuarty/
COPY licenses/ /usr/share/doc/stuarty/licenses/
COPY --from=build /out/stuarty /usr/local/bin/stuarty

# A container binds every interface, so the image runs in network mode (SDD-007
# 4.3 item 4). APP_URL is the loopback case, the only one served over plain
# HTTP; any other use needs TLS. The operator supplies APP_KEY (32 bytes or
# more, base64) at docker run: a key baked into the image would be public.
ENV HOST=0.0.0.0 \
	PORT=7878 \
	STUARTY_MODE=network \
	APP_URL=http://localhost:7878 \
	STUARTY_HOME=/data \
	STUARTY_FFMPEG=/opt/ffmpeg/bin/ffmpeg \
	STUARTY_FFPROBE=/opt/ffmpeg/bin/ffprobe \
	PATH=/opt/ffmpeg/bin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
VOLUME ["/data"]
USER app
WORKDIR /data
EXPOSE 7878
HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
	CMD curl -fsS http://127.0.0.1:7878/up || exit 1
ENTRYPOINT ["stuarty"]
CMD ["serve"]
