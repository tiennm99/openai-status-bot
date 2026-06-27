FROM golang:1.25-alpine AS build

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/openai-status-bot ./cmd/openai-status-bot

FROM alpine:3.21
RUN adduser -D -H appuser
USER appuser
COPY --from=build /out/openai-status-bot /usr/local/bin/openai-status-bot
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD wget -q -O /dev/null http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["openai-status-bot"]
