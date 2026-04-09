FROM alpine:3.23
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/websudo /usr/bin
ENTRYPOINT ["/usr/bin/websudo"]
