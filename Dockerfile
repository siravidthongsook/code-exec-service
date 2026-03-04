FROM golang:1.23-bookworm AS build

WORKDIR /app
COPY go.mod .
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o /out/code-exec-service ./cmd/server

FROM debian:bookworm AS nsjail-builder
RUN apt-get update && apt-get install -y \
    bison \
    flex \
    g++ \
    git \
    libnl-route-3-dev \
    libprotobuf-dev \
    make \
    pkg-config \
    protobuf-compiler \
    && rm -rf /var/lib/apt/lists/*

RUN git clone --depth 1 --branch 3.1 https://github.com/google/nsjail.git /nsjail
WORKDIR /nsjail
RUN make

FROM debian:bookworm-slim
RUN apt-get update \
  && apt-get install -y --no-install-recommends \
     ca-certificates \
     python3 \
     nodejs \
     default-jdk-headless \
     gcc \
     g++ \
     golang-go \
     libnl-route-3-200 \
     libprotobuf32 \
  && rm -rf /var/lib/apt/lists/*

COPY --from=nsjail-builder /nsjail/nsjail /usr/bin/nsjail

WORKDIR /app
COPY --from=build /out/code-exec-service /app/code-exec-service
COPY nsjail /app/nsjail

ENV CODE_EXEC_USE_NSJAIL=1
ENV CODE_EXEC_NSJAIL_CONFIG=/app/nsjail/nsjail.cfg

EXPOSE 7070
EXPOSE 9090
CMD ["/app/code-exec-service"]
