# syntax=docker/dockerfile:1

FROM node:24-alpine AS web
WORKDIR /src
COPY web/package*.json ./web/
RUN npm --prefix web ci
COPY web ./web
COPY internal/frontend ./internal/frontend
RUN npm --prefix web run build

FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/frontend/dist ./internal/frontend/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/lemail ./cmd/lemail

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/lemail /app/lemail
COPY config/config.example.json /app/config/config.example.json
ENV CONFIG_PATH=/app/config/config.json
EXPOSE 3000 2525
ENTRYPOINT ["/app/lemail"]

