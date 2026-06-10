FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /auto-fix .

FROM alpine:3.19
RUN apk add --no-cache git nodejs npm maven ca-certificates curl tar

# Download the Xray-Lib plugin at image build time and place it in PATH.
# Both PrepareGenerator() and GetLocalXrayLibExecutablePath() check PATH first,
# so this prevents any runtime download attempt entirely.
ARG XRAY_LIB_VERSION=1.3.0
RUN curl -fsSL \
    "https://releases.jfrog.io/artifactory/xray-scan-lib/xray-scan-lib-${XRAY_LIB_VERSION}-linux-amd64.tar.gz" \
    | tar -xz -C /tmp/ \
    && mv /tmp/xray-scan-lib-${XRAY_LIB_VERSION}-linux-amd64/xray-scan-plugin /usr/local/bin/xray-scan-plugin \
    && chmod +x /usr/local/bin/xray-scan-plugin \
    && rm -rf /tmp/xray-scan-lib-*

COPY --from=builder /auto-fix /usr/local/bin/auto-fix
ENTRYPOINT ["auto-fix"]
