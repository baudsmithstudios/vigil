#!/bin/sh
set -e

IMAGE="vigil:latest"

# Verify the image is loaded.
if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
    echo "Error: Docker image '$IMAGE' not found." >&2
    echo "Load it first:  docker load < vigil.tar" >&2
    exit 1
fi

case "${1:-}" in
    init|mounts)
        docker run --rm -it \
            --pid=host \
            -v "$(pwd)":/work \
            -v /proc:/proc:ro \
            -v /sys:/sys:ro \
            -w /work \
            "$IMAGE" "$@"
        ;;
    *)
        echo "Usage: vigil.sh <command>"
        echo ""
        echo "Commands:"
        echo "  init     Generate config.toml interactively"
        echo "  mounts   Discover and configure mount watchdog"
        echo ""
        echo "Options are passed through (e.g. ./vigil.sh init -config custom.toml)"
        echo ""
        echo "To run the daemon, use docker compose:"
        echo "  docker compose up -d"
        echo "  docker attach vigil"
        ;;
esac
