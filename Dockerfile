FROM golang:1.16 AS builder
WORKDIR /go/src/github.com/makedevops/api-server/
COPY go.mod .
COPY app.go .
RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /go/src/github.com/makedevops/api-server/app .
EXPOSE 8080
CMD ["./app"]

