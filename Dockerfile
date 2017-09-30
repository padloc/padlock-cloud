FROM golang:alpine

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

RUN addgroup -g 1000 -S padlock-cloud &&\
    adduser -u 1000 -D -S -G padlock-cloud padlock-cloud

RUN mkdir -p ${WORKDIR} && mkdir -p ${WORKDIR}/db && mkdir -p ${WORKDIR}/logs &&\
    touch ${WORKDIR}/logs/info.log && touch ${WORKDIR}/logs/error.log

WORKDIR ${WORKDIR}

RUN apk --no-cache add git su-exec && go get github.com/maklesoft/padlock-cloud &&\
    apk del git

COPY . ${WORKDIR}

VOLUME ["$WORKDIR/assets", "$WORKDIR/db", "$WORKDIR/logs"]

ENTRYPOINT ["/bin/sh", "./docker-entrypoint.sh"]

EXPOSE 8080 8443

CMD ["padlock-cloud", "runserver"]