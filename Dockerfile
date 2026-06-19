FROM golang:1.24-alpine AS build

WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/openai-status-bot ./cmd/openai-status-bot

FROM alpine:3.21
RUN adduser -D -H appuser
USER appuser
COPY --from=build /out/openai-status-bot /usr/local/bin/openai-status-bot
ENTRYPOINT ["openai-status-bot"]
