# Build builder container
FROM golang:1.18.1-alpine3.15 as builder

RUN apk add --no-cache git

COPY . /src
WORKDIR /src

RUN go build -o /reverse-hash-service

# Build running container
FROM alpine:3.15.4

COPY --from=builder /reverse-hash-service /bin/reverse-hash-service
COPY --from=builder /src/schema.sql /opt/schema.sql

RUN apk add --no-cache ca-certificates tzdata
RUN adduser -D -g '' appuser
USER appuser

ENTRYPOINT ["/bin/reverse-hash-service"]
