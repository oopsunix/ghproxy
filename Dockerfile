FROM golang:1.23.2
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go get -d -v ./
RUN CGO_ENABLED=0 go build -a -installsuffix cgo -o ghproxy .
EXPOSE 45000
CMD ["./ghproxy"]