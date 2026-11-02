# Multi-stage Go build — produces ~15MB image
FROM golang:alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/scheduler ./cmd/scheduler

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/scheduler /bin/scheduler
ENTRYPOINT ["/bin/scheduler"]
