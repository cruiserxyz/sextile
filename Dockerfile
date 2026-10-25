# Sextile in a container: Astrolog 8.00 compiled from a checksum-pinned
# archive, the Go binary, and nothing else. Build and run: README, Install and run.

# --- Astrolog -----------------------------------------------------------------
FROM debian:bookworm-slim AS astrolog
ARG ASTROLOG_VERSION=8.00
ARG ASTROLOG_SOURCE_ARCHIVE=ast80src.zip
ARG ASTROLOG_SOURCE_SHA256=8c0c33eac2c40d158c007f3a4168516972c9fc78830f65eba38ac2be46541d63
# The 1200-1800 ephemeris files: Alois Treindl's swisseph repository at a pinned
# commit, one file each, so the URLs cannot change under the checksums.
ARG SWISSEPH_COMMIT=c69c14adb7a0f94829586ffd92cd422aacb21e86
ARG SEPL_12_SHA256=8dccace2557601a223d5f8a7cf64de4e6bbb8a82b9b130b95a54d49adfbe546d
ARG SEMO_12_SHA256=1c65fdbb854f350d3de36afc3e4ac126a53f360ae4f6dcb57beed7c690e7eb61
ARG SEAS_12_SHA256=3345555097e2080f4e2582026a6008ebbccfdca27600ba48019ca9c6e3e7a211
RUN apt-get update \
 && apt-get install -y --no-install-recommends curl ca-certificates unzip build-essential \
 && rm -rf /var/lib/apt/lists/*
# The archive is verified BEFORE it is unzipped; a changed upstream file stops
# the build here.
RUN set -eux \
 && install -d /tmp/astrolog-source \
 && curl -fsSL -o "/tmp/${ASTROLOG_SOURCE_ARCHIVE}" \
      "https://www.astrolog.org/ftp/${ASTROLOG_SOURCE_ARCHIVE}" \
 && printf '%s  %s\n' "${ASTROLOG_SOURCE_SHA256}" "/tmp/${ASTROLOG_SOURCE_ARCHIVE}" \
      | sha256sum -c - \
 && unzip -q "/tmp/${ASTROLOG_SOURCE_ARCHIVE}" -d /tmp/astrolog-source \
 && cd /tmp/astrolog-source \
 && sed -i 's|^#define X11|//#define X11|' astrolog.h \
 && sed -i 's|^#define DEFAULT_DIR ""|#define DEFAULT_DIR "/usr/local/share/astrolog"|' astrolog.h \
 && sed -i 's|-lX11||' Makefile \
 && make -j"$(nproc)" \
 && install -m 0755 astrolog /usr/local/bin/astrolog \
 && install -d /usr/local/share/astrolog \
 && install -m 0644 astrolog.as atlas.as timezone.as /usr/local/share/astrolog/ \
 && install -m 0644 sefstars.txt seorbel.txt /usr/local/share/astrolog/ \
 && install -m 0644 ephem/sepl_18.se1 ephem/semo_18.se1 ephem/seas_18.se1 \
      /usr/local/share/astrolog/ \
 && install -m 0644 ephem/se00010s.se1 ephem/se50000s.se1 ephem/se90377s.se1 \
      ephem/se90482s.se1 ephem/s136108s.se1 ephem/s136199s.se1 \
      ephem/s136472s.se1 ephem/s225088s.se1 /usr/local/share/astrolog/ \
 && cd / \
 && rm -rf /tmp/astrolog-source "/tmp/${ASTROLOG_SOURCE_ARCHIVE}"
# The 1200-1800 planet, Moon and main-asteroid files, for charts before 1800.
# Each is verified before it is installed.
RUN set -eux \
 && for f in sepl_12:"${SEPL_12_SHA256}" semo_12:"${SEMO_12_SHA256}" seas_12:"${SEAS_12_SHA256}"; do \
      n="${f%%:*}.se1"; \
      curl -fsSL -o "/tmp/${n}" \
        "https://raw.githubusercontent.com/aloistr/swisseph/${SWISSEPH_COMMIT}/ephe/${n}" \
      && printf '%s  %s\n' "${f#*:}" "/tmp/${n}" | sha256sum -c - \
      && install -m 0644 "/tmp/${n}" /usr/local/share/astrolog/ \
      && rm "/tmp/${n}"; \
    done
# The build fails unless this really is Astrolog 8.00 and Swiss Ephemeris
# loaded: a silently broken ephemeris path puts every planet at 0Ari00. Do NOT
# pass -b here; it is a toggle and would undo the =b the init file applies.
RUN set -eux \
 && astrolog -Hc > /tmp/astrolog-version.txt \
 && grep -F "Astrolog version ${ASTROLOG_VERSION}" /tmp/astrolog-version.txt \
 && astrolog -qa 6 15 1990 14:30 5:00 122W 37N > /tmp/chart.txt \
 && ! grep -q "not found" /tmp/chart.txt \
 && grep -q "^Sun :.*Gem" /tmp/chart.txt \
 && ! grep -q "^Sun :  0Ari00" /tmp/chart.txt \
 && astrolog -qa 6 15 1500 12:00 0:00 0W 51N -R0 11 > /tmp/old.txt \
 && ! grep -q "not found" /tmp/old.txt \
 && astrolog -qa 6 15 1990 14:30 5:00 122W 37N -R0 43 > /tmp/hygiea.txt \
 && ! grep -q "not found" /tmp/hygiea.txt \
 && rm /tmp/chart.txt /tmp/astrolog-version.txt /tmp/old.txt /tmp/hygiea.txt

# --- Sextile ------------------------------------------------------------------
FROM golang:1.25-bookworm AS gobuild
ARG SEXTILE_VERSION=docker
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY pkg/ pkg/
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=${SEXTILE_VERSION}" -o /out/sextile ./cmd/sextile

# --- Final image --------------------------------------------------------------
FROM debian:bookworm-slim AS final
COPY --from=astrolog /usr/local/bin/astrolog /usr/local/bin/astrolog
COPY --from=astrolog /usr/local/share/astrolog /usr/local/share/astrolog
COPY --from=gobuild /out/sextile /usr/local/bin/sextile
USER 65532:65532
ENV SEXTILE_CHARTS_DIR=/charts \
    SEXTILE_PREVIEW_MODE=serve \
    SEXTILE_PREVIEW_ADDR=0.0.0.0:5307 \
    XDG_DATA_HOME=/charts/.sextile/data \
    XDG_CONFIG_HOME=/charts/.sextile/config \
    SEXTILE_ATLAS_PATH=/usr/local/share/astrolog/atlas.as
EXPOSE 5307
ENTRYPOINT ["sextile"]
