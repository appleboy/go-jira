FROM alpine:3.20

ARG TARGETOS
ARG TARGETARCH

LABEL maintainer="Bo-Yi Wu <appleboy.tw@gmail.com>" \
  org.label-schema.name="Jira API CLI" \
  org.label-schema.vendor="Bo-Yi Wu" \
  org.label-schema.schema-version="1.0"

LABEL org.opencontainers.image.source=https://github.com/appleboy/go-jira
LABEL org.opencontainers.image.description="Jira API CLI"
LABEL org.opencontainers.image.licenses=MIT

RUN apk add --no-cache ca-certificates && \
  rm -rf /var/cache/apk/* && \
  addgroup -g 1000 jira && \
  adduser -D -u 1000 -G jira jira

COPY release/${TARGETOS}/${TARGETARCH}/go-jira /bin/
RUN chown jira:jira /bin/go-jira

USER jira

ENTRYPOINT ["/bin/go-jira"]
