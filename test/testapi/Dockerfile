FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o testapi .

FROM alpine:3.20
RUN adduser -D appuser
WORKDIR /app
COPY --from=builder /app/testapi .
USER appuser
EXPOSE 8081
CMD ["./testapi"]
