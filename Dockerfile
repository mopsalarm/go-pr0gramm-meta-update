FROM golang:1.9.2-alpine3.6 as builder

RUN apk add --no-cache git ca-certificates
RUN go get github.com/Masterminds/glide/

COPY glide.* src/github.com/mopsalarm/go-pr0gramm-meta-update/
RUN cd src/github.com/mopsalarm/go-pr0gramm-meta-update/ && glide install --strip-vendor

COPY . src/github.com/mopsalarm/go-pr0gramm-meta-update/
RUN go build -v -ldflags="-s -w" -o /go-pr0gramm-meta-update github.com/mopsalarm/go-pr0gramm-meta-update


FROM alpine:3.6
RUN apk add --no-cache ca-certificates

COPY --from=builder /go-pr0gramm-meta-update /
ENTRYPOINT ["/go-pr0gramm-meta-update"]
