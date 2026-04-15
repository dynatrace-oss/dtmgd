#!/bin/bash
export IMAGE_NAME=dtmgd-ai-container
export SSH_SCOPE_DIR=/home/ihudak/.ssh
export EXTRA_MOUNTS="/home/ihudak/dev/dtctl:ro /home/ihudak/dev/dynatrace-managed-mcp:ro /home/ihudak/dev/dtmgd.copilot"
./runme.sh build
# ./runme.sh restricted /home/ihudak/dev/dtmgd
./runme.sh discovery /home/ihudak/dev/dtmgd
