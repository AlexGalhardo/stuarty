# The Vukem Video image: the vukem-video binary (server, CLI and the UI assets)
# and a pinned LGPL FFmpeg. Published to ghcr.io/alexgalhardo/vukem-video and
# alexgalhardo/vukem-video (Docker Hub) by release.yml; see
# docs/sdds/009-releases-and-ci-cd.md section 11. Build from the repository root:
#
#   docker build -f .config/docker/vukem-video.Dockerfile -t vukem-video .
#
# Run it on your machine only (the published port is bound to loopback):
#
#   docker run --rm -p 127.0.0.1:7878:7878 -v vukem-video:/data vukem-video
#
# No web server runs here: the Go binary serves HTTP. The product bundles and
# links no GPL component: .config/ffmpeg.env pins the BtbN lgpl build (ADR-0011);
# the packages of the Debian base are listed with their licences.
ARG BUN_IMAGE=oven/bun:1.4.4-alpine
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
# The .git directory travels in the context so Go stamps the tag at HEAD into
# the binary, which `vukem-video version` prints. A release, or image.yml with a
# local dummy tag, passes VERSION and the build fails when the stamped version is
# not v$VERSION: that also catches a dirty build context (+dirty). From a git
# worktree the .git file points outside the context and git fails; the build then
# goes without VCS stamping and the check is skipped, as it is for VERSION=devel.
RUN if git -c safe.directory=/src status --porcelain >/dev/null 2>&1; then \
		git config --global --add safe.directory /src; stamp=true; \
	else stamp=false; fi \
	&& GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -buildvcs="$stamp" -trimpath -ldflags="-s -w" -o /out/vukem-video ./web/cmd/app \
	&& if [ "$stamp" = true ] && [ "$VERSION" != devel ] && [ "$TARGETARCH" = "$(go env GOHOSTARCH)" ]; then \
		got="$(/out/vukem-video version)"; \
		[ "$got" = "vukem-video v$VERSION" ] || { echo "version stamp: got [$got], want [vukem-video v$VERSION]" >&2; exit 1; }; \
	fi

# The pinned FFmpeg, downloaded by exact URL and verified by SHA-256.
FROM --platform=$BUILDPLATFORM ${RUNTIME_IMAGE} AS ffmpeg
ARG TARGETARCH
COPY .config/ffmpeg.env /tmp/ffmpeg.env
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl xz-utils \
	&& rm -rf /var/lib/apt/lists/* \
	&& . /tmp/ffmpeg.env \
	&& case "$TARGETARCH" in \
		amd64) asset="$FFMPEG_ASSET_LINUX_AMD64"; sum="$FFMPEG_SHA256_LINUX_AMD64" ;; \
		arm64) asset="$FFMPEG_ASSET_LINUX_ARM64"; sum="$FFMPEG_SHA256_LINUX_ARM64" ;; \
		*) echo "unsupported architecture: $TARGETARCH" >&2; exit 1 ;; \
	esac \
	&& curl -fsSL -o /tmp/ffmpeg.tar.xz "https://github.com/BtbN/FFmpeg-Builds/releases/download/${FFMPEG_RELEASE_TAG}/${asset}" \
	&& echo "${sum}  /tmp/ffmpeg.tar.xz" | sha256sum -c - \
	&& mkdir -p /opt/ffmpeg \
	&& tar -xJf /tmp/ffmpeg.tar.xz -C /opt/ffmpeg --strip-components=1 \
	&& test -x /opt/ffmpeg/bin/ffmpeg && test -x /opt/ffmpeg/bin/ffprobe && test -s /opt/ffmpeg/LICENSE.txt

FROM ${RUNTIME_IMAGE}
ARG VERSION=devel
ARG REVISION=unknown

LABEL org.opencontainers.image.title="vukem-video" \
	org.opencontainers.image.description="Vukem Video: a minimal video editor operated by AI agents and by a human UI" \
	org.opencontainers.image.source="https://github.com/AlexGalhardo/vukem-video" \
	org.opencontainers.image.url="https://github.com/AlexGalhardo/vukem-video" \
	org.opencontainers.image.documentation="https://github.com/AlexGalhardo/vukem-video#readme" \
	org.opencontainers.image.licenses="MIT" \
	org.opencontainers.image.version="${VERSION}" \
	org.opencontainers.image.revision="${REVISION}"

# curl is the health check client; tzdata and ca-certificates are the runtime
# data the binary needs; util-linux provides prlimit, which the child process
# limits of P2-RND-005 use. Debian slim keeps /usr/share/doc/<package>/copyright.
RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl tzdata util-linux \
	&& rm -rf /var/lib/apt/lists/* \
	&& useradd --system --no-create-home --uid 10001 --user-group app \
	&& mkdir -p /data /usr/share/doc/vukem-video/ffmpeg \
	&& chown app:app /data \
	&& dpkg-query -W > /usr/share/doc/vukem-video/base-image-packages.txt

COPY --from=ffmpeg /opt/ffmpeg/bin/ffmpeg /opt/ffmpeg/bin/ffprobe /opt/ffmpeg/bin/
COPY --from=ffmpeg /opt/ffmpeg/LICENSE.txt /usr/share/doc/vukem-video/ffmpeg/LICENSE.txt
COPY .config/ffmpeg.env /tmp/ffmpeg.env
# The licence obligations of the LGPL build (ADR-0011 item 2): the licence text,
# the configure line and where the corresponding source is.
RUN . /tmp/ffmpeg.env \
	&& /opt/ffmpeg/bin/ffmpeg -version | sed -n "s/^configuration: //p" > /usr/share/doc/vukem-video/ffmpeg/configure.txt \
	&& test -s /usr/share/doc/vukem-video/ffmpeg/configure.txt \
	&& ! grep -q -e "--enable-gpl" -e "--enable-nonfree" /usr/share/doc/vukem-video/ffmpeg/configure.txt \
	&& printf "%s\n%s\n%s\n" \
		"FFmpeg ${FFMPEG_VERSION} (LGPL build) from https://github.com/BtbN/FFmpeg-Builds/releases/tag/${FFMPEG_RELEASE_TAG}" \
		"Build scripts: https://github.com/BtbN/FFmpeg-Builds/tree/${FFMPEG_BUILDS_COMMIT}" \
		"Corresponding FFmpeg source: ${FFMPEG_SOURCE_URL}" > /usr/share/doc/vukem-video/ffmpeg/SOURCE.txt \
	&& rm /tmp/ffmpeg.env

COPY LICENSE.md THIRD_PARTY_NOTICES.md /usr/share/doc/vukem-video/
COPY licenses/ /usr/share/doc/vukem-video/licenses/
COPY --from=build /out/vukem-video /usr/local/bin/vukem-video

# A container binds every interface, so the image runs in network mode (SDD-007
# 4.3 item 4). APP_URL is the loopback case, the only one served over plain
# HTTP; any other use needs TLS. The operator supplies APP_KEY (32 bytes or
# more, base64) at docker run: a key baked into the image would be public.
ENV HOST=0.0.0.0 \
	PORT=7878 \
	VUKEM_VIDEO_MODE=network \
	APP_URL=http://localhost:7878 \
	VUKEM_VIDEO_HOME=/data \
	VUKEM_VIDEO_FFMPEG=/opt/ffmpeg/bin/ffmpeg \
	VUKEM_VIDEO_FFPROBE=/opt/ffmpeg/bin/ffprobe \
	PATH=/opt/ffmpeg/bin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
VOLUME ["/data"]
USER app
WORKDIR /data
EXPOSE 7878
HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
	CMD curl -fsS http://127.0.0.1:7878/up || exit 1
ENTRYPOINT ["vukem-video"]
CMD ["serve"]
