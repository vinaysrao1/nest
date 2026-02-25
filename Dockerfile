FROM golang:1-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /nest ./cmd/server/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /migrate ./cmd/migrate/

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
RUN addgroup -S nest && adduser -S nest -G nest
WORKDIR /app
COPY --from=builder /nest .
COPY --from=builder /migrate .
COPY migrations/ ./migrations/
USER nest
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s CMD wget -qO- http://localhost:8080/api/v1/health || exit 1
ENTRYPOINT ["/app/nest"]
