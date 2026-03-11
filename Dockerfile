FROM alpine:3.21

RUN apk add --no-cache tzdata \
    && addgroup -S changemonitor \
    && adduser -S -G changemonitor changemonitor \
    && mkdir -p /config /data \
    && chown changemonitor:changemonitor /config /data

COPY changemonitor /usr/bin/changemonitor

USER changemonitor

ENV CONFIG_FILE=/config/config.json
ENV STORAGE_DIRECTORY=/data
ENV CHROME_WS=ws://127.0.0.1:9222
ENV ENABLE_WEBUI=false

ENTRYPOINT ["/usr/bin/changemonitor"]