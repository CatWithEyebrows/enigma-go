package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const optionsTestSpec = `{
  "openrpc": "1.0.0",
  "info": {"title": "QIX", "version": "0.0.1"},
  "methods": [{
    "name": "Doc.Check",
    "description": "Checks an expression.",
    "params": [
      {"name": "qExpr", "description": "Expression to check.", "required": true, "schema": {"type": "string"}},
      {"name": "qLabels", "description": "List of labels.", "schema": {"type": "array", "items": {"type": "string"}}},
      {"name": "qStrict", "description": "Strict validation.", "schema": {"type": "boolean"}},
      {"name": "qPage", "description": "A page.", "schema": {"$ref": "#/components/schemas/NxPage"}}
    ],
    "result": {"name": "Results", "schema": {"type": "object", "properties": {"qErrorMsg": {"type": "string"}, "qNames": {"type": "array", "items": {"type": "string"}}}}}
  }],
  "components": {"schemas": {"NxPage": {"type": "object", "properties": {"qTop": {"type": "integer"}}}}}
}`

// Calls the generated methods through a mock socket and prints the parameters each call sends
const optionsTestMain = `package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/CatWithEyebrows/enigma-go/v4"
)

func main() {
	ctx := context.Background()
	socket, _ := enigma.NewMockSocket("")
	dialer := enigma.Dialer{
		CreateSocket: func(context.Context, string, http.Header) (enigma.Socket, error) { return socket, nil },
		Interceptors: []enigma.Interceptor{func(ctx context.Context, invocation *enigma.Invocation, next enigma.InterceptorContinuation) *enigma.InvocationResponse {
			params, _ := json.Marshal(invocation.Params)
			fmt.Println(string(params))
			return &enigma.InvocationResponse{Result: json.RawMessage(` + "`" + `{"qErrorMsg":"ok","qNames":["n"]}` + "`" + `)}
		}},
	}
	remoteObject, err := dialer.DialRaw(ctx, "ws://mock", nil)
	if err != nil {
		panic(err)
	}
	doc := &Doc{remoteObject}
	strict := true
	doc.Check(ctx, "x")
	doc.CheckWithOptions(ctx, "x", nil)
	doc.CheckWithOptions(ctx, "x", &DocCheckOptions{})
	doc.CheckWithOptions(ctx, "x", &DocCheckOptions{Labels: []string{"a"}})
	doc.CheckWithOptions(ctx, "x", &DocCheckOptions{Strict: &strict})
	doc.CheckRawWithOptions(ctx, "x", &DocCheckRawOptions{Page: map[string]int{"qTop": 1}})
	msg, names, _ := doc.CheckWithOptions(ctx, "x", nil)
	fmt.Println(msg, names)
}
`

func TestFrozenParamsGenerateWithOptions(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	spec := write("spec.json", optionsTestSpec)
	companion := write("schema-companion.json", `{"remote-object-return-types": {}}`)
	write("frozen-params.json", `{"Doc.Check": 1, "Doc.Removed": 2}`)
	generated := filepath.Join(dir, "generated.go")
	if err := generate(spec, companion, generated, "main", false); err != nil {
		t.Fatal(err)
	}

	frozen, err := loadFrozenParams(filepath.Join(dir, "frozen-params.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(frozen) != "map[Doc.Check:1]" {
		t.Errorf("frozen params not updated, got %v", frozen)
	}

	// Run from the module root so the generated code can import enigma
	cmd := exec.Command(goTool, "run", generated, write("main.go", optionsTestMain))
	cmd.Dir = ".."
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("running generated code failed: %v\n%s", err, output)
	}
	expected := []string{
		`["x"]`,
		`["x"]`,
		`["x"]`,
		`["x",["a"]]`,
		`["x",[],true]`,
		`["x",[],false,{"qTop":1}]`,
		`["x"]`,
		`ok [n]`,
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if strings.Join(lines, "\n") != strings.Join(expected, "\n") {
		t.Errorf("unexpected params sent\ngot:\n%s\nexpected:\n%s", strings.Join(lines, "\n"), strings.Join(expected, "\n"))
	}
}

func TestUpdateFrozenParams(t *testing.T) {
	method := func(name string, params int) *OpenRpcMethod {
		return &OpenRpcMethod{Name: name, Parameters: make([]*Type, params)}
	}
	frozen := map[string]int{"Doc.Same": 2, "Doc.Grown": 1, "Doc.Shrunk": 3, "Doc.Gone": 1}
	hasOptions := updateFrozenParams(frozen, []*OpenRpcMethod{
		method("Doc.Same", 2), method("Doc.Grown", 2), method("Doc.Shrunk", 1), method("Doc.New", 4),
	})
	got, _ := json.Marshal(frozen)
	if string(got) != `{"Doc.Grown":1,"Doc.New":4,"Doc.Same":2,"Doc.Shrunk":1}` || !hasOptions {
		t.Errorf("got %s, hasOptions=%v", got, hasOptions)
	}
}

func TestFormatComment(t *testing.T) {
	text := "Reloads the script that is set in an app.\n\n### Logs\nWhen this method is called, audit activity logs are produced to track the user activity.\nIn the case of errors, both audit activity logs and system services logs are produced.\nThe log files are named as follows:\n<table>\n<tr>\n<th>Audit activity log</th>\n<th>System service log</th>\n</tr>\n<tr>\n<td>&lt;i&gt;&lt; MachineName&gt;&lt;/i&gt;AuditActivity&lt;i&gt;Engine.txt&lt;/i&gt; in Qlik Sense Enterprise&lt;br&gt;&lt;i&gt;&lt; MachineName&gt;&lt;/i&gt;AuditActivity&lt;i&gt;Engine.log&lt;/i&gt; in Qlik Sense Desktop</td>\n<td>&lt;i&gt;&lt; MachineName&gt;&lt;/i&gt;Service&lt;i&gt;Engine.txt&lt;/i&gt; in Qlik Sense Enterprise&lt;br&gt;&lt;i&gt;&lt; MachineName&gt;&lt;/i&gt;Service&lt;i&gt;Engine.log&lt;/i&gt; in Qlik Sense Desktop</td>\n</tr>\n</table>\n\n### Where to find the log files\nThe location of the log files depends on whether you have installed Qlik Sense Enterprise or Qlik Sense Desktop.\n<table>\n<tr>\n<th>Qlik Sense Enterprise </th>\n<th>Qlik Sense Desktop </th>\n</tr>\n<tr>\n<td>&lt;i&gt;%ProgramData%/Qlik/Sense/Log/Engine&lt;/i&gt;</td>\n<td>&lt;i&gt;%UserProfile%/Documents/Qlik/Sense/Log&lt;/i&gt;</td>\n</tr>\n</table>"

	fmt.Println(text)
	fmt.Println("-----------------------------------------")
	fmt.Println(formatComment(" ", text, []*Type{}))
}

func TestFormatComment2(t *testing.T) {
	text := "Returns:\n* The list of tables in an app and the fields inside each table.\n* The list of derived fields.\n* The list of key fields."

	fmt.Println(text)
	fmt.Println("-----------------------------------------")
	fmt.Println(formatComment(" ", text, []*Type{}))
}
