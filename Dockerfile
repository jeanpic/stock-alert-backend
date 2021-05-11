FROM golang:1.15.12-alpine3.13

WORKDIR /go/src/stock-alert-backend
COPY . .

RUN go get -d -v ./...
RUN go install -v ./...
RUN go build cmd/stock-alert-backend/main.go

FROM alpine:3.13
COPY --from=0 /go/src/stock-alert-backend/main .
CMD ["./main"]

EXPOSE 8000