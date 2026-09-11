# Celln host-installer image.
#
# Runs celln's scripts/setup-host.sh inside the privileged celln-installer
# DaemonSet to install the host dispatcher on KVM-capable nodes. This is NOT
# the distroless `celln` image: setup-host.sh needs a shell plus host tooling
# (nsenter, openssl, skopeo, python3; systemctl is reached via nsenter on the
# host) and a Rust toolchain that it copies into the host for the forge step.
#
# The Celln image (version, archive checksum and image digest) is pinned in
# config/celln/release.json. CELLN_IMAGE is passed in by CI and the Makefile;
# it is not hardcoded here so the pin stays in one place.
ARG CELLN_IMAGE
FROM ${CELLN_IMAGE} AS celln

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
        bash \
        ca-certificates \
        curl \
        openssl \
        python3 \
        skopeo \
        util-linux \
    && rm -rf /var/lib/apt/lists/*

# The celln CLI and runtime assets (including scripts/setup-host.sh).
COPY --from=celln /usr/local/bin/celln /usr/local/bin/celln
COPY --from=celln /opt/celln/runtime /opt/celln/runtime

# Preinstall the Rust toolchain so setup-host.sh can copy it into /host rather
# than reaching the network on first boot. The musl std target is included, but
# no musl linker/cc is installed here: the dispatcher's forge step links on the
# *host*, which must provide its own musl-gcc/cc. See docs/concepts/celln-backend.md.
RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y --default-toolchain stable --profile minimal \
    && /root/.cargo/bin/rustup target add x86_64-unknown-linux-musl

ENTRYPOINT ["/bin/bash", "/opt/celln/runtime/scripts/setup-host.sh"]
