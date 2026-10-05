FROM golang:1.27.1 AS builder

WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o api ./


FROM scratch
WORKDIR /app
COPY --from=builder /app/api ./api
CMD ["./api"]