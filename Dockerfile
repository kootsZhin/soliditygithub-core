# syntax=docker/dockerfile:1

FROM golang:1.19.4

WORKDIR /

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY .env ./
COPY *.go ./
RUN go build -o /main

CMD ["/main"]