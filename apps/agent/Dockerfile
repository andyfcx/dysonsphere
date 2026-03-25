FROM golang:1.23-alpine AS builder
WORKDIR /workspace

COPY go.work go.work.sum ./
COPY apps/server/go.mod apps/server/go.sum ./apps/server/
COPY apps/agent/go.mod apps/agent/go.sum ./apps/agent/
COPY packages/shared/go.mod ./packages/shared/
RUN cd apps/agent && go mod download

COPY apps/agent ./apps/agent
COPY packages/shared ./packages/shared
RUN CGO_ENABLED=0 GOOS=linux go build -o /observer-agent ./apps/agent/cmd/agent

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /observer-agent .
ENTRYPOINT ["./observer-agent"]
CMD ["run", "--config", "/etc/observer-agent/config.yaml"]
