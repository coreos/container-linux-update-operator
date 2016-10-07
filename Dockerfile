FROM alpine:3.4

RUN apk add --no-cache ca-certificates

COPY bin /bin/

ENTRYPOINT ["/bin/klocksmith"]

