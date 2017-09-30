#!/usr/bin/env bash
set -e

if [ "${1:0:1}" = '-' ]; then
	set -- padlock-cloud runserver "$@"
fi

if [ "$1" = 'padlock-cloud' ]; then
    chown -R 1000:1000 .
    su-exec padlock-cloud "$@"
    exit 0
fi

exec "$@"