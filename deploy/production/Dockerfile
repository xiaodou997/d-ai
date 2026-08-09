FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata \
    && mkdir -p /data/images /data/files

COPY --chmod=0555 release/dai-linux-amd64 /app/dai

VOLUME ["/data"]
EXPOSE 19641

ENTRYPOINT ["/app/dai"]
