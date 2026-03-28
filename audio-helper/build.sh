#!/bin/bash
set -euo pipefail
cd "$(dirname "$0")"
swift build -c release
mkdir -p ../bin
cp .build/release/heimdall-audio ../bin/heimdall-audio
echo "Built heimdall-audio -> bin/heimdall-audio"
