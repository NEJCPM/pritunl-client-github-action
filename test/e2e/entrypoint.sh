#!/bin/bash
set -e

if [ -n "$MONGODB_URI" ]; then
  pritunl set-mongodb "$MONGODB_URI"
fi

exec pritunl start --conf /etc/pritunl.conf
