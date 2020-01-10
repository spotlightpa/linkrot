#!/bin/bash

set -eux -o pipefail

# Get the directory that this script file is in
THIS_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

cd "$THIS_DIR"


function main() {
    ARGUMENT="${1:-redis-start}"

    case "$ARGUMENT" in
    redis-start)
        docker run --name redis-container --rm -p 6379:6379 redis:latest
        ;;

    redis-cli)
        docker run --name redis-cli -it --rm \
            --link redis-container:redis \
            redis:latest \
            redis-cli -h redis -p 6379
        ;;

    *)
        echo "Unknown argument: $ARGUMENT"
        exit 1
        ;;
    esac
}

main "$@"
