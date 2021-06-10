FROM alpine
COPY changemonitor /usr/bin/changemonitor
RUN apk add --no-cache tzdata
ENV CONFIG_FILE=/config/config.json
ENV STORAGE_DIRECTORY=/data
ENV CHROME_WS=ws://127.0.0.1:9222
ENV ENABLE_WEBUI=false
ENTRYPOINT ["/usr/bin/changemonitor"]