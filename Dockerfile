FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /auto-fix .

# Use a glibc-based runtime — the Xray-Lib plugin binary is dynamically linked
# against glibc and will fail with "no such file or directory" on Alpine (musl).
FROM ubuntu:22.04
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    nodejs \
    npm \
    maven \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Download the Xray-Lib plugin at image build time and place it in PATH.
ARG XRAY_LIB_VERSION=1.3.0
RUN curl -fsSL \
    "https://releases.jfrog.io/artifactory/xray-scan-lib/xray-scan-lib-${XRAY_LIB_VERSION}-linux-amd64.tar.gz" \
    | tar -xz -C /tmp/ \
    && find /tmp/xray-scan-lib-* -name "xray-scan-plugin" -exec mv {} /usr/local/bin/xray-scan-plugin \; \
    && chmod +x /usr/local/bin/xray-scan-plugin \
    && rm -rf /tmp/xray-scan-lib-*

COPY --from=builder /auto-fix /usr/local/bin/auto-fix
ENTRYPOINT ["auto-fix"]
