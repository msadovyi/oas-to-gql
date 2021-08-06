#!/bin/sh

SPEC_PATH=${1:-1}
echo "Running oas/$SPEC_PATH"

node "oas/$SPEC_PATH/index.js" &
P1=$!
go run . --path="oas/$SPEC_PATH/spec.json" &
P2=$!
wait $P1 $P2
