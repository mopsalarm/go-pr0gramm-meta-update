FROM centurylink/ca-certs
COPY go-pr0gramm-meta-update /
ENTRYPOINT ["/go-pr0gramm-meta-update"]
