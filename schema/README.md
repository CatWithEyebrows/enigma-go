# Schema Generator

This folder contains a script for generating enigma-go based on a specific version of Qlik Associative Engine.
The Engine OpenRPC specification can be found in the schema folder [`./schema/engine-rpc.json`](./engine-rpc.json).

To generate enigma-go from the latest published Engine specification, run:
```sh
./schema/generate.sh
```
The script downloads the spec from https://qlik.dev/specs/json-rpc/qix.json into `./schema/engine-rpc.json`
and regenerates `qix_generated.go` if it differs from `origin/master`. Set `QIX_SPEC_URL` to use another source.

## Stable method signatures

Go has no optional parameters, so when the engine adds an optional parameter to an existing method, regenerating
would change the method's signature and break callers. To avoid that, [`frozen-params.json`](./frozen-params.json)
records how many parameters each method takes. The generator keeps that many in the method's signature, and puts
parameters added later into a `<Method>WithOptions` variant that takes an options struct:

```go
// Unchanged
func (obj *Doc) CheckExpression(ctx context.Context, expr string, labels []string, strictFieldValidation bool) (...)

// Added when a later spec adds e.g. qMaxErrors
type DocCheckExpressionOptions struct {
	MaxErrors *int
}
func (obj *Doc) CheckExpressionWithOptions(ctx context.Context, expr string, labels []string, strictFieldValidation bool, opts *DocCheckExpressionOptions) (...)
```

Options left `nil` are not sent, so the engine uses its defaults. An unset option before a set one is sent as its
zero value, or as `[]`/`{}` for arrays and objects, since the engine rejects `null`.

The generator maintains `frozen-params.json`: new methods are added with all their parameters, and methods removed
from the spec are dropped. To deliberately change a signature, for example to move an option into the method itself,
edit its count in the file and regenerate.
