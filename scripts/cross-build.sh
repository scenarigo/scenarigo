#!/bin/bash -e
# This file is used in the golang-cross container.

if [ -d "/scenarigo" ]; then
    git config --global --add safe.directory /scenarigo
fi


cmd="bash /entrypoint.sh --skip=publish --clean --verbose"
if [ "${SNAPSHOT}" != "" ]; then
    echo "snapshot build"
    cmd="${cmd} --snapshot"
fi
eval ${cmd}
