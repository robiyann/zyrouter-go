FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY frontend/ ./frontend/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o zyrouter ./cmd/zyrouter/

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /app/zyrouter /usr/local/bin/zyrouter
COPY --from=builder /app/frontend /opt/zyrouter/frontend
ENV FRONTEND_DIR=/opt/zyrouter/frontend
EXPOSE 20128
ENTRYPOINT ["zyrouter"]
