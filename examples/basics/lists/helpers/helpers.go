package main

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"os"

	"github.com/CatWithEyebrows/enigma-go/v4"
)

const script = `
Orders:
Load
RecNo() as OrderID,
Date(MakeDate(2024, 1, 1) + RecNo()) as OrderDate
AutoGenerate 100;
`

func main() {
	// Fetch the QCS_HOST and QCS_API_KEY from the environment variables
	qcsHost := os.Getenv("QCS_HOST")
	qcsApiKey := os.Getenv("QCS_API_KEY")

	// Connect to Qlik Cloud tenant and create a session document:
	ctx := context.Background()
	global, err := enigma.Dialer{}.Dial(ctx, fmt.Sprintf("wss://%s/app/SessionApp_%v", qcsHost, rand.Int()), http.Header{
		"Authorization": []string{fmt.Sprintf("Bearer %s", qcsApiKey)},
	})
	if err != nil {
		fmt.Println("Not able to connect", err)
		panic(err)
	}
	defer global.DisconnectFromServer()

	doc, _ := global.GetActiveDoc(ctx)
	doc.SetScript(ctx, script)
	doc.DoReload(ctx, 0, false, false)

	// Add a sheet so the sheet list has something in it
	doc.CreateObjectRaw(ctx, map[string]any{
		"qInfo":    map[string]any{"qType": "sheet"},
		"qMetaDef": map[string]any{"title": "Overview"},
		"rank":     1,
	})

	// The list helpers create a session object for the list the first time and reuse it after that
	fields, err := doc.GetFieldList(ctx)
	if err != nil {
		panic(err)
	}
	for _, field := range fields {
		fmt.Printf("Field %s in tables %v\n", field.Name, field.SrcTables)
	}

	sheets, err := doc.GetSheetList(ctx)
	if err != nil {
		panic(err)
	}
	for _, sheet := range sheets {
		fmt.Printf("Sheet %s: %q with rank %v\n", sheet.Info.Id, sheet.Data.Title, sheet.Data.Rank)
	}

	// The list object versions return the session object, so you can follow changes to the list
	sheetList, _ := doc.GetSheetListObject(ctx)
	changed := sheetList.ChangedChannel()
	doc.CreateObjectRaw(ctx, map[string]any{
		"qInfo":    map[string]any{"qType": "sheet"},
		"qMetaDef": map[string]any{"title": "Details"},
		"rank":     2,
	})
	<-changed
	sheets, _ = sheetList.Items(ctx)
	fmt.Printf("The sheet list changed and now has %d sheets\n", len(sheets))
}
