FROM alpine:3.18

# TARGETOS and TARGETARCH are set automatically when --platform is provided.
ARG TARGETOS
ARG TARGETARCH
ARG NAME
ARG NAME_PREFIX

ADD "./bin/${NAME}/${TARGETOS}_${TARGETARCH}/${NAME_PREFIX}-${NAME}" "/app"

ENTRYPOINT ["/app"]