FROM golang:1.26.3-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /icon-svg ./cmd/icon-svg

FROM alpine:3.23
RUN apk add --no-cache su-exec && addgroup -g 10001 app && adduser -D -u 10001 -G app app && mkdir -p /app/data && chown app:app /app/data
WORKDIR /app
COPY --from=build /icon-svg /app/icon-svg
COPY web /app/web
COPY --chown=10001:10001 library /app/library
COPY THIRD_PARTY_NOTICES.txt /app/
COPY --chmod=755 docker-entrypoint.sh /app/docker-entrypoint.sh
ENV LISTEN_ADDR=0.0.0.0:8000 DATA_DIR=/app/data LIBRARY_DIR=/app/library WEB_DIR=/app/web OPEN_BROWSER=false
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 CMD ["/app/icon-svg", "-healthcheck"]
ENTRYPOINT ["/app/docker-entrypoint.sh"]
