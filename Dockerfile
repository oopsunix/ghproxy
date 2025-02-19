FROM golang:1.23.2 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o ghproxy -ldflags="-s -w" .

FROM alpine:latest
RUN apk --no-cache add ca-certificates curl grep
ENV TZ=Asia/Shanghai
WORKDIR /app
COPY --from=builder /app/ghproxy /app/ghproxy
COPY --from=builder /app/public /app/public
COPY --from=builder /app/config.json /app/config.json
EXPOSE 45000
ENTRYPOINT ["/app/ghproxy"]