FROM alpine:3.20

ARG TARGETOS
ARG TARGETARCH

# Broker listen port, used by EXPOSE and the HEALTHCHECK probe below.
ARG BROKER_PORT=8080

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

# Persist the port into the container so the runtime HEALTHCHECK shell can read it
# (build ARGs are not visible at runtime).
ENV BROKER_PORT=${BROKER_PORT}

# Default listen port for `go-jira broker serve` (override with --listen / JIRA_BROKER_LISTEN).
EXPOSE ${BROKER_PORT}

# Liveness probe against the broker's /healthz endpoint (BusyBox wget ships with alpine).
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --quiet --tries=1 --spider "http://127.0.0.1:${BROKER_PORT}/healthz" || exit 1

ENTRYPOINT ["/bin/go-jira"]
