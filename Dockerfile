FROM alpine:3.6

RUN apk add --no-cache ca-certificates

COPY bin /bin/

ENTRYPOINT ["/bin/update-agent"]

