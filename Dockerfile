FROM golang:1.14-alpine as builder

WORKDIR /app

COPY ./go.mod .
COPY ./go.sum .

RUN go mod download

COPY . .

RUN go build -o ./out/control-plane *.go

# -----------------------------------------------

FROM alpine:3.12.0

COPY --from=builder /app/out/control-plane .
# COPY --from=builder /app/control-plane/migrations /migrations

EXPOSE 18000

ENTRYPOINT ["./control-plane"]