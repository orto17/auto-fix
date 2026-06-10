FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /auto-fix .

FROM alpine:3.19
RUN apk add --no-cache git nodejs npm maven ca-certificates
COPY --from=builder /auto-fix /usr/local/bin/auto-fix
ENTRYPOINT ["auto-fix"]
