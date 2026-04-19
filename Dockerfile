FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /swarmex-admission ./cmd

FROM alpine:3.21
RUN apk add --no-cache ca-certificates wget iptables
COPY --from=build /swarmex-admission /usr/local/bin/swarmex-admission
EXPOSE 8080
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD wget -qO- http://localhost:8080/health || exit 1
ENTRYPOINT ["swarmex-admission"]
