# Build image
FROM golang:alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -o ./out/app .

# Runtime image
FROM alpine
COPY --from=build /app/out/app /usr/local/bin/changemonitor
RUN apk add --no-cache tzdata
ENV CONFIG_FILE=/config/config.json
ENV STORAGE_DIRECTORY=/data
ENV CHROME_WS=ws://127.0.0.1:9222
ENV ENABLE_WEBUI=false
ENTRYPOINT ["/usr/local/bin/changemonitor"]