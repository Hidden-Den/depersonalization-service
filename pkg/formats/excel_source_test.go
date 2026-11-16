package formats_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"dep_service/pkg/anonymizer"
	"dep_service/pkg/formats"
)

func saveSimpleWorkbook(t *testing.T, path, fio string) {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	_ = f.SetSheetRow("Sheet1", "A1", &[]interface{}{"ФИО", "Телефон"})
	_ = f.SetSheetRow("Sheet1", "A2", &[]interface{}{fio, "+7 999 123-45-67"})
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save workbook: %v", err)
	}
}

// When input_path no longer points at the file the caller opened (replaced on disk),
// the handler must refuse instead of anonymizing different contents.
func TestExcelHandler_InputPathReplacedAfterOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.xlsx")
	saveSimpleWorkbook(t, path, "Иванов Иван Иванович")

	in, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()

	// Replace the path with another workbook while the original handle stays open.
	other := filepath.Join(dir, "other.xlsx")
	saveSimpleWorkbook(t, other, "Петров Пётр Петрович")
	if err := os.Rename(other, path); err != nil {
		t.Skipf("cannot replace an open file on this platform: %v", err)
	}

	var out bytes.Buffer
	err = formats.NewExcelHandler(anonymizer.New("salt")).Process(context.Background(), in, &out,
		map[string]interface{}{"input_path": path})
	if err == nil || !strings.Contains(err.Error(), "modified or replaced") {
		t.Fatalf("expected source replacement error, got %v", err)
	}
}

// Without an explicit input_path an *os.File reader is consumed as a reader,
// even if its name no longer resolves to a file.
func TestExcelHandler_OSFileWithoutInputPathUsesReader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.xlsx")
	saveSimpleWorkbook(t, path, "Иванов Иван Иванович")

	in, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	if err := os.Remove(path); err != nil {
		t.Skipf("cannot remove an open file on this platform: %v", err)
	}

	var out bytes.Buffer
	if err := formats.NewExcelHandler(anonymizer.New("salt")).Process(context.Background(), in, &out, nil); err != nil {
		t.Fatalf("expected reader-based processing to succeed, got %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("expected anonymized workbook output")
	}
}

// The normal path-based flow (as used by the processor) still works.
func TestExcelHandler_InputPathMatchesReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.xlsx")
	saveSimpleWorkbook(t, path, "Иванов Иван Иванович")

	in, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()

	var out bytes.Buffer
	err = formats.NewExcelHandler(anonymizer.New("salt")).Process(context.Background(), in, &out,
		map[string]interface{}{"input_path": path})
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	res, err := excelize.OpenReader(&out)
	if err != nil {
		t.Fatalf("open result: %v", err)
	}
	defer res.Close()
	if v, _ := res.GetCellValue("Sheet1", "A2"); v == "" || v == "Иванов Иван Иванович" {
		t.Fatalf("expected anonymized FIO, got %q", v)
	}
}
