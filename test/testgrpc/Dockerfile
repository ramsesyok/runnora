FROM golang:1.26-alpine3.24 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o testgrpc .

FROM alpine:3.24
RUN adduser -D appuser
WORKDIR /app
COPY --from=builder /app/testgrpc .
USER appuser
EXPOSE 50051
CMD ["./testgrpc"]
