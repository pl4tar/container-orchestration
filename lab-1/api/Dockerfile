FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/ ./...

FROM alpine:3.20 AS runtime
RUN apk add --no-cache ca-certificates tzdata && adduser -D -u 10001 app
WORKDIR /app
COPY --from=build /out/api /app/
RUN chown -R app /app
USER app
EXPOSE 8080
ENTRYPOINT ["/app/api"]