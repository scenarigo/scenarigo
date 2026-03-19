#!/bin/sh

set -eu

BASEDIR=`dirname $0`/..

if [ "$(git symbolic-ref --short HEAD)" != "main" ]; then
    echo "branch is not main"
    exit 1
fi

git diff --quiet || (echo "diff exists"; exit 1)

VERSION=$(gobump show -r ${BASEDIR}/version)
echo "current version: ${VERSION}"
read -p "input next version: " NEXT_VERSION
gobump set ${NEXT_VERSION} -w ${BASEDIR}/version

git-chglog --next-tag v${NEXT_VERSION} -o ${BASEDIR}/CHANGELOG.md

GO_VERSION=$(go version | perl -waln -e 'print $F[2]')
perl -pi -e "s/go_version='.+'/go_version='${GO_VERSION}'/" README.md
perl -pi -e "s/@v${VERSION}/@v${NEXT_VERSION}/" README.md

read -p "release v${NEXT_VERSION}? (y/N): " yn
case "$yn" in
  [yY]*) ;;
  *) echo abort; exit 1;;
esac

git commit -am "release v${NEXT_VERSION}"
git tag v${NEXT_VERSION}
git push && git push --tags
