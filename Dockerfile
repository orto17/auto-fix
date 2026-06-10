FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /auto-fix .

FROM alpine:3.19
RUN apk add --no-cache git nodejs npm maven ca-certificates

# GitHub Actions sets HOME to /github/home which can be read-only inside Docker.
# Use a directory we own so the Xray-Lib plugin can be downloaded and cached there.
ENV HOME=/root

COPY --from=builder /auto-fix /usr/local/bin/auto-fix
ENTRYPOINT ["auto-fix"]
