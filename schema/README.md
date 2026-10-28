# Schema Generator

This folder contains a script for generating enigma-go based on a specific version of Qlik Associative Engine.
The Engine OpenRPC specification can be found in the schema folder [`./schema/engine-rpc.json`](./engine-rpc.json).

To generate enigma-go from the latest published Engine specification, run:
```sh
./schema/generate.sh
```
The script downloads the spec from https://qlik.dev/specs/json-rpc/qix.json into `./schema/engine-rpc.json`
and regenerates `qix_generated.go` if it differs from `origin/master`. Set `QIX_SPEC_URL` to use another source.
