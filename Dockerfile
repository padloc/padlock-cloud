FROM golang:alpine as padlock-cloud

WORKDIR /go/src/github.com/padlock/padlock-cloud

COPY . .

RUN apk update && apk --no-cache add git
    
RUN go get -u github.com/kardianos/govendor && \
    govendor sync

RUN CGO_ENABLED=0 GOOS=linux go build -a -v -installsuffix cgo -o padlock-cloud main.go

FROM alpine:latest

ARG user=padlock-cloud
ARG uid=1000
ARG group=padlock-cloud
ARG gid=1000

ARG WORKDIR=/opt/padlock-cloud

ENV PC_CONFIG_PATH="" \
    PC_LOG_FILE=${WORKDIR}/logs/info.log \
    PC_ERR_FILE=${WORKDIR}/logs/error.log \
    PC_NOTIFY_ERRORS="" \
    PC_LEVELDB_PATH=${WORKDIR}/db \
    PC_EMAIL_SERVER="" \
    PC_EMAIL_PORT="" \
    PC_EMAIL_USER="" \
    PC_EMAIL_PASSWORD="" \
    PC_PORT=8080 \
    PC_ASSETS_PATH=${WORKDIR}/assets \
    PC_TLS_CERT="" \
    PC_TLS_KEY="" \
    PC_BASE_URL=http://localhost \
    PC_CORS=1 \
    PC_TEST=0

RUN addgroup -g ${gid} -S ${group} &&\
    adduser -u ${uid} -D -S -G ${group} ${user}

RUN mkdir -p ${WORKDIR} && mkdir -p ${WORKDIR}/db && mkdir -p ${WORKDIR}/logs &&\
    mkdir -p ${WORKDIR}/ssl && touch ${WORKDIR}/logs/info.log && touch ${WORKDIR}/logs/error.log

RUN apk update && apk --no-cache add su-exec ca-certificates

WORKDIR ${WORKDIR}

COPY --from=padlock-cloud /go/src/github.com/padlock/padlock-cloud/padlock-cloud /usr/local/bin
COPY docker-entrypoint.sh .
COPY assets ./assets

VOLUME ["$WORKDIR/assets", "$WORKDIR/db", "$WORKDIR/logs", "$WORKDIR/ssl"]

ENTRYPOINT ["/bin/sh", "./docker-entrypoint.sh"]

EXPOSE 8080 8443

CMD ["padlock-cloud", "runserver"]
