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
ENV CONFIG_FILE=/config/config.json
ENV STORAGE_DIRECTORY=/data
ENTRYPOINT ["/usr/local/bin/changemonitor"]