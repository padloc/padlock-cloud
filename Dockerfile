FROM golang:alpine as padlock-cloud

WORKDIR /go/src/github.com/maklesoft/padlock-cloud

COPY . .

RUN apk update && apk --no-cache add git
    
RUN go get -u github.com/kardianos/govendor && \
    govendor sync

RUN CGO_ENABLED=0 GOOS=linux go build -a -v -installsuffix cgo -o padlock-cloud main.go

FROM golang:alpine as caddy

RUN apk update && apk --no-cache add git && \
    go get github.com/mholt/caddy/caddy

WORKDIR /go/src/github.com/mholt/caddy/caddy

RUN CGO_ENABLED=0 GOOS=linux go build -a -v -installsuffix cgo -o caddy main.go

RUN ls -la

FROM alpine:latest

ARG user=padlock-cloud
ARG uid=1000
ARG group=padlock-cloud
ARG gid=1000

ARG WORKDIR=/opt/padlock-cloud

ENV PC_CONFIG_PATH ""

ENV PC_LOG_FILE ${WORKDIR}/logs/info.log
ENV PC_ERR_FILE ${WORKDIR}/logs/error.log
ENV PC_NOTIFY_ERRORS ""

ENV PC_LEVELDB_PATH ${WORKDIR}/db

ENV PC_EMAIL_SERVER ""
ENV PC_EMAIL_PORT ""
ENV PC_EMAIL_USER ""
ENV PC_EMAIL_PASSWORD ""

ENV PC_PORT 8080

ENV PC_ASSETS_PATH ${WORKDIR}/assets

ENV PC_TLS_CERT ""
ENV PC_TLS_KEY ""

ENV PC_BASE_URL http://localhost

ENV PC_CORS 1

ENV PC_TEST 0

RUN addgroup -g ${gid} -S ${group} &&\
    adduser -u ${uid} -D -S -G ${group} ${user}

RUN mkdir -p ${WORKDIR} && mkdir -p ${WORKDIR}/db && mkdir -p ${WORKDIR}/logs &&\
    mkdir -p ${WORKDIR}/ssl && touch ${WORKDIR}/logs/info.log && touch ${WORKDIR}/logs/error.log

RUN apk update && apk --no-cache add su-exec

WORKDIR ${WORKDIR}

COPY --from=padlock-cloud /go/src/github.com/maklesoft/padlock-cloud/padlock-cloud /usr/local/bin
COPY --from=caddy /go/src/github.com/mholt/caddy/caddy/caddy /usr/local/bin
COPY docker-entrypoint.sh .
COPY assets ./assets

VOLUME ["$WORKDIR/assets", "$WORKDIR/db", "$WORKDIR/logs", "$WORKDIR/ssl"]

ENTRYPOINT ["/bin/sh", "./docker-entrypoint.sh"]

EXPOSE 8080 8443

CMD ["padlock-cloud", "runserver"]