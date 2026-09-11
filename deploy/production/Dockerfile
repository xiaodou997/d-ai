FROM debian:bookworm-slim

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata wget \
    && rm -rf /var/lib/apt/lists/* \
    && addgroup --system --gid 10001 dai \
    && adduser --system --uid 10001 --ingroup dai --no-create-home --disabled-login dai \
    && mkdir -p /data/images /data/files \
    && chown -R dai:dai /data

COPY --chmod=0555 release/dai-linux-amd64 /app/dai
COPY release/LICENSE release/NOTICE release/THIRD-PARTY-LICENSES.md /usr/share/licenses/dai/
COPY release/TRADEMARKS.md release/COMMERCIAL_LICENSE.md /usr/share/licenses/dai/

VOLUME ["/data"]
EXPOSE 19641

# The production compose file also drops all Linux capabilities and uses a
# read-only root filesystem. The application only needs the mounted data
# volume for mutable state.
USER 10001:10001

ENTRYPOINT ["/app/dai"]
