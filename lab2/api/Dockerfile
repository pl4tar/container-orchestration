FROM golang:1.25 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o main ./


FROM scratch
WORKDIR /app
COPY --from=builder /app/main ./main
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["./main"]
