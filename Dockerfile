FROM golang:1.23-alpine AS builder
ENV GOTOOLCHAIN=auto
RUN apk add --no-cache git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/api      ./cmd/api \
 && CGO_ENABLED=0 go build -o /bin/worker   ./cmd/worker \
 && CGO_ENABLED=0 go build -o /bin/reaper   ./cmd/reaper \
 && CGO_ENABLED=0 go build -o /bin/migrate  ./cmd/migrate

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/api     /usr/local/bin/api
COPY --from=builder /bin/worker  /usr/local/bin/worker
COPY --from=builder /bin/reaper  /usr/local/bin/reaper
COPY --from=builder /bin/migrate /usr/local/bin/migrate
COPY migrations/ /migrations/
