#!/bin/bash -eu

root_dir=$(cd $(dirname $0)/../; pwd)

if [ $# -ne 2 ]; then
    echo "usage ./scripts/bump-go-version.sh prev-ver latest-ver"
    exit 1
fi
prev=$1
latest=$2

prev_major_minor=$(echo ${prev} | perl -wlne '/(\d+.\d+).*/ and print $1')

for p in $(find ${root_dir} -name 'go.mod'); do
    echo ${p}
    cd $(dirname ${p})
    go mod edit -go=${prev_major_minor}
    if [[ ${p} =~ ${root_dir}/examples/ ]]; then
        go mod edit -toolchain=go${latest}
    fi
done

perl -pi -e "s/semver\.NewVersion\(\"\d*\.\d*.0\"\)/semver\.NewVersion\(\"${prev_major_minor}.0\"\)/" ${root_dir}/scripts/cross-build/build-matrix/main.go
