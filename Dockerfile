FROM node:22-alpine AS ui
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY tsconfig.json ./
COPY web ./web
COPY scripts ./scripts
RUN mkdir -p internal/battle && npm run typecheck && npm run build:ui

FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /app/internal/battle/battlefield.html ./internal/battle/battlefield.html
RUN CGO_ENABLED=0 go build -o /TotalBotWar ./cmd/server

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && adduser -D app && mkdir -p /data && chown app:app /data
COPY --from=builder /TotalBotWar /usr/local/bin/TotalBotWar
USER app
ENV HOST=0.0.0.0 PORT=3005 DATA_DIR=/data
EXPOSE 3005
VOLUME ["/data"]
ENTRYPOINT ["TotalBotWar"]
