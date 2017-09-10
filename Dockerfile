FROM golang:alpine

EXPOSE 3000

RUN apk --no-cache add git && go get github.com/maklesoft/padlock-cloud && apk del git

ENTRYPOINT padlock-cloud  --db-path=/db runserver --cors
