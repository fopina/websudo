FROM alpine:3.24
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/websudo /usr/bin
ENTRYPOINT ["/usr/bin/websudo"]
