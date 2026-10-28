#!/bin/bash

## compares engine spec with master

## the public spec on qlik.dev, override with QIX_SPEC_URL
QIX_SPEC_URL=${QIX_SPEC_URL:-https://qlik.dev/specs/json-rpc/qix.json}

generate() {
  local tmp
  tmp=$(mktemp)
  if ! curl -sSf "$QIX_SPEC_URL" | jq . > "$tmp" || ! jq -e '.info.version and (.methods | length > 0)' "$tmp" >/dev/null; then
    echo "Failed to download a valid spec from $QIX_SPEC_URL"
    rm -f "$tmp"
    return 1
  fi
  mv "$tmp" ./schema/engine-rpc.json
  git diff --exit-code origin/master -- ./schema/engine-rpc.json >/dev/null
  if [[ $? -ne 0 ]]; then
    ENGINE_VERSION=$(cat ./schema/engine-rpc.json | jq -r '.info.version')
    echo "Generating enigma-go based on OPEN-RPC API for Qlik Associative Engine version $ENGINE_VERSION"
    ## generate code
    go run ./schema/generate.go ./schema/engine-rpc.json ./schema/schema-companion.json ./qix_generated.go enigma disable-enigma-import && \
      go fmt ./qix_generated.go >/dev/null
  else
    echo "No changes to engine-rpc.json, nothing to do."
  fi
}

generate
