FROM golang:1.23-alpine AS builder
WORKDIR /workspace

# Copy workspace manifests first for layer caching
COPY go.work go.work.sum ./
COPY apps/server/go.mod apps/server/go.sum ./apps/server/
COPY apps/agent/go.mod apps/agent/go.sum ./apps/agent/
COPY packages/shared/go.mod ./packages/shared/
RUN cd apps/server && go mod download

# Copy source trees needed for this binary
COPY apps/server ./apps/server
COPY packages/shared ./packages/shared
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./apps/server/cmd/server

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /server .
COPY --from=builder /workspace/apps/server/migrations ./migrations
EXPOSE 8000
CMD ["./server"]
