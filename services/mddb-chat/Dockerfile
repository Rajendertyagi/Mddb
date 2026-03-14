# Build stage
FROM rust:1.82-alpine AS builder

RUN apk add --no-cache musl-dev protobuf-dev protoc

WORKDIR /app

# Copy proto definitions
COPY proto/ /proto/

# Copy source
COPY services/mddb-chat/ .

# Update proto path for Docker context
RUN sed -i 's|../../proto/|/proto/|g' build.rs

RUN cargo build --release

# Runtime stage
FROM alpine:3.22

RUN apk add --no-cache ca-certificates && \
    addgroup -S mddb && adduser -S mddb -G mddb

COPY --from=builder /app/target/release/mddb-chat /usr/local/bin/mddb-chat

USER mddb

EXPOSE 11030

HEALTHCHECK --interval=15s --timeout=3s --start-period=5s \
  CMD wget -q --spider http://localhost:11030/health || exit 1

ENTRYPOINT ["mddb-chat"]
