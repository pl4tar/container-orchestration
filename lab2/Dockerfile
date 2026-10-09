FROM golang:1.27.1 AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o api ./

FROM alpine:3.22
WORKDIR /app
COPY  --from=builder /app/api ./
EXPOSE 8080

CMD ["./api"]
