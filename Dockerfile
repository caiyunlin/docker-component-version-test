######## Stage 1: Build
FROM golang:latest AS build
WORKDIR /app
COPY --link . .
RUN go mod download && CGO_ENABLED=0 GOOS=linux go build -o main .

# Print Go and module versions
RUN echo "======== Stage 1: Build Versions ========" && \
    echo "Go version:  $(go version)" && \
    echo "" && \
    echo "Go module dependencies:" && \
    go list -m all && \
    echo "=========================================="

######## Stage 2: Run (Alpine)
FROM alpine:latest AS release-alpine
WORKDIR /app
COPY --link --from=build /app/main .

RUN apk add --no-cache ca-certificates openssl && \
    echo "======== Stage 2: Alpine Versions ========" && \
    echo "Alpine version:      $(cat /etc/alpine-release)" && \
    echo "ca-certificates:     $(apk info -v ca-certificates 2>/dev/null)" && \
    echo "openssl version:     $(openssl version)" && \
    echo "=========================================="

CMD ["./main"]

######## Stage 3: Run (Ubuntu)
FROM ubuntu:noble AS release-ubuntu
WORKDIR /app
COPY --link --from=build /app/main .

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        apt-transport-https \
        ca-certificates \
        software-properties-common \
        httping \
        man \
        man-db \
        vim \
        screen \
        curl \
        gnupg \
        atop \
        htop \
        jq \
        dnsutils \
        tcpdump \
        traceroute \
        iputils-ping \
        iptables \
        net-tools \
        ncat \
        iproute2 \
        strace \
        telnet \
        openssl \
        psmisc \
        dsniff \
        mtr-tiny \
        conntrack \
        ethtool \
        iputils-tracepath \
        lsof \
        nmap \
        socat \
        sysstat \
        wget && \
    rm -rf /var/lib/apt/lists/*

# Print installed component versions
RUN echo "======== Stage 3: Ubuntu Versions ========" && \
    echo "Ubuntu version:      $(cat /etc/os-release | grep PRETTY_NAME | cut -d= -f2)" && \
    echo "curl:                $(curl --version | head -1)" && \
    echo "openssl:             $(openssl version)" && \
    echo "vim:                 $(vim --version | head -1)" && \
    echo "jq:                  $(jq --version)" && \
    echo "nmap:                $(nmap --version | head -1)" && \
    echo "socat:               $(socat -V | head -2 | tail -1)" && \
    echo "tcpdump:             $(tcpdump --version 2>&1 | head -1)" && \
    echo "traceroute:          $(traceroute --version 2>&1 | head -1)" && \
    echo "htop:                $(htop --version | head -1)" && \
    echo "wget:                $(wget --version | head -1)" && \
    echo "ca-certificates:     $(dpkg -s ca-certificates 2>/dev/null | grep Version)" && \
    echo "=========================================="

CMD ["./main"]
