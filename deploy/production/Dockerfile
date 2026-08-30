FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S -g 10001 dai \
    && adduser -S -D -H -u 10001 -G dai dai \
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
