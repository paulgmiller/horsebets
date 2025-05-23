FROM golang:1.23 as build

WORKDIR /go/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY *.go ./
COPY templates/*.html ./templates/

RUN go test -v

RUN go build -o /go/bin/app

FROM gcr.io/distroless/base

COPY --from=build /go/bin/app /
COPY templates/*.html /templates/
CMD ["/app"]
