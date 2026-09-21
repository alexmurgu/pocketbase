# Build the PocketBase binary.
FROM golang:alpine AS builder-golang
WORKDIR /directory-to-build-golang-app
# Download dependencies separately to reuse this layer.
COPY [ "go.mod", "go.sum", "./" ]
RUN go mod download
# Copy the source and build the binary.
COPY . .
RUN cd examples/base && go build -o pocketbase ./

# Build the runtime image.
FROM alpine:latest
LABEL org.opencontainers.image.source="https://github.com/alexmurgu/pocketbase"
LABEL org.opencontainers.image.description="PocketBase with PostgreSQL support"
RUN apk add --no-cache ca-certificates postgresql-client

# Copy the PocketBase binary from the build stage.
COPY --from=builder-golang /directory-to-build-golang-app/examples/base/pocketbase /pb/pocketbase

# Uncomment to include a local pb_migrations directory.
# COPY ./pb_migrations /pb/pb_migrations

# Uncomment to include a local pb_hooks directory.
# COPY ./pb_hooks /pb/pb_hooks

EXPOSE 8080

# Start PocketBase.
CMD ["/pb/pocketbase", "serve"]
