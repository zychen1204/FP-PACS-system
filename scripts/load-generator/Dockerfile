FROM golang:1.21-alpine AS builder

WORKDIR /src

COPY go.mod ./
COPY *.go ./

RUN CGO_ENABLED=0 GOOS=linux go build -o /out/load-generator .

FROM postgres:16-alpine

COPY --from=builder /out/load-generator /usr/local/bin/load-generator

WORKDIR /work

ENTRYPOINT ["load-generator"]
