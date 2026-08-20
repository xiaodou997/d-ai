FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata \
    && mkdir -p /data/images /data/files

COPY --chmod=0555 release/dai-linux-amd64 /app/dai
COPY release/LICENSE release/NOTICE release/THIRD-PARTY-LICENSES.md /usr/share/licenses/dai/
COPY release/TRADEMARKS.md release/COMMERCIAL_LICENSE.md /usr/share/licenses/dai/

VOLUME ["/data"]
EXPOSE 19641

ENTRYPOINT ["/app/dai"]
