FROM alpine:3.23
ARG TARGETPLATFORM
# TODO: update binary name
COPY $TARGETPLATFORM/golang-template /usr/bin
ENTRYPOINT ["/usr/bin/golang-template"]