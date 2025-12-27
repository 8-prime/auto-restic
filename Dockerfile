FROM golang:1.25 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY main.go ./main.go

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/app ./main.go

FROM restic/restic:latest

COPY --from=builder /out/app ./app

ENTRYPOINT ["/app"]