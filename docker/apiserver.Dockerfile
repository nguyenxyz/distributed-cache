FROM golang:1.23 AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o apiserver ./cmd/apiserver

FROM alpine:latest

COPY --from=builder /app/apiserver /apiserver

EXPOSE 4000 8000

ENTRYPOINT ["/apiserver"]
