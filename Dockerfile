FROM golang:alpine

EXPOSE 3000

RUN apk --no-cache add git && go get github.com/maklesoft/padlock-cloud && apk del git

ENTRYPOINT padlock-cloud --notify-errors ${NOTIFY_EMAIL}  --db-path=/db --email-server ${MAIL_SERVER} --email-port ${MAIL_PORT} --email-user ${MAIL_USER} --email-password ${MAIL_PASSWORD} runserver --cors --base-url ${BASE_URL}
