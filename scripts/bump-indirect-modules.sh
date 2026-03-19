#!/bin/sh

set -eu

BASEDIR=`dirname $0`/..

cd ${BASEDIR}
cat go.mod | perl -ale 'print $F[0] if $_ =~ /indirect$/' | xargs -I{} go get {}@latest
go mod tidy
