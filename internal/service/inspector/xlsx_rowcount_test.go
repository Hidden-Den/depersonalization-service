package inspector_test

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/xuri/excelize/v2"

	"dep_service/internal/domain"
	"dep_service/internal/service/inspector"
)

var reSheetDimension = regexp.MustCompile(`<dimension ref="[^"]*"\s*(/>|></dimension>)`)

// writeXLSX builds a single-sheet workbook. firstRow is the 1-based row of the header,
// preamble rows (one text cell each) are written above it. If dimension is non-empty the
// worksheet <dimension> ref is rewritten to it, emulating metadata from other producers.
func writeXLSX(t *testing.T, preamble, firstRow, dataRows int, dimension string) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.xlsx")

	f := excelize.NewFile()
	for i := 1; i <= preamble; i++ {
		_ = f.SetCellValue("Sheet1", fmt.Sprintf("A%d", i), fmt.Sprintf("Служебная строка отчёта №%d", i))
	}
	_ = f.SetSheetRow("Sheet1", fmt.Sprintf("A%d", firstRow), &[]interface{}{"ФИО", "Телефон", "Email"})
	for i := 1; i <= dataRows; i++ {
		row := []interface{}{
			fmt.Sprintf("Иванов Иван Иванович %d", i),
			fmt.Sprintf("+7 999 123-%02d-%02d", i/100, i%100),
			fmt.Sprintf("user%d@example.com", i),
		}
		_ = f.SetSheetRow("Sheet1", fmt.Sprintf("A%d", firstRow+i), &row)
	}
	if err := f.SaveAs(src); err != nil {
		t.Fatalf("save xlsx: %v", err)
	}
	_ = f.Close()
	if dimension == "" {
		return src
	}

	return patchWorksheetXML(t, src, func(data []byte) []byte {
		if !reSheetDimension.Match(data) {
			t.Fatalf("worksheet has no <dimension> element to patch")
		}
		return reSheetDimension.ReplaceAll(data, []byte(`<dimension ref="`+dimension+`"/>`))
	})
}

// patchWorksheetXML copies the workbook at src into a new file, rewriting
// xl/worksheets/sheet1.xml with edit. Every archive operation is checked so a broken
// fixture fails the test instead of producing misleading results.
func patchWorksheetXML(t *testing.T, src string, edit func([]byte) []byte) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "patched.xlsx")
	zr, err := zip.OpenReader(src)
	if err != nil {
		t.Fatalf("open source zip: %v", err)
	}
	defer zr.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create patched zip: %v", err)
	}
	zw := zip.NewWriter(out)
	for _, zf := range zr.File {
		rc, err := zf.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", zf.Name, err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read zip entry %s: %v", zf.Name, err)
		}
		if zf.Name == "xl/worksheets/sheet1.xml" {
			data = edit(data)
		}
		w, err := zw.Create(zf.Name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", zf.Name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("write zip entry %s: %v", zf.Name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("finalize patched zip: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close patched zip: %v", err)
	}
	return dst
}

func inspectXLSXFile(t *testing.T, path string) *domain.FileInfo {
	t.Helper()
	info, err := inspector.New().Inspect(context.Background(), path)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	return info
}

// A long preamble leaves only a few data rows inside the 40-row header sample;
// the preview must still be filled with 10 rows read past the sample.
func TestInspectXLSX_LongPreambleFillsPreview(t *testing.T) {
	path := writeXLSX(t, 35, 36, 30, "")
	info := inspectXLSXFile(t, path)

	if info.RowCount != 30 {
		t.Errorf("expected 30 data rows, got %d", info.RowCount)
	}
	if len(info.PreviewRows) != 10 {
		t.Fatalf("expected 10 preview rows, got %d", len(info.PreviewRows))
	}
	if got := info.PreviewRows[9].Before[0]; got != "Иванов Иван Иванович 10" {
		t.Errorf("10th preview row should come from past the sample, got %q", got)
	}
}

// A consistent dimension is used as-is; with a header on row 5 the data rows are 6..60.
func TestInspectXLSX_DimensionWithLeadingEmptyRows(t *testing.T) {
	path := writeXLSX(t, 0, 5, 55, "A5:C60")
	info := inspectXLSXFile(t, path)

	if info.RowCount != 55 {
		t.Errorf("expected 55 data rows, got %d", info.RowCount)
	}
	if len(info.PreviewRows) != 10 {
		t.Errorf("expected 10 preview rows, got %d", len(info.PreviewRows))
	}
}

// A stale dimension that ends inside the already-read sample is not trusted.
func TestInspectXLSX_StaleDimensionFallsBackToExactCount(t *testing.T) {
	path := writeXLSX(t, 0, 1, 100, "A1:C5")
	info := inspectXLSXFile(t, path)

	if info.RowCount != 100 {
		t.Errorf("expected exact count of 100 data rows, got %d", info.RowCount)
	}
}

// Without a usable dimension (excelize writes a single-cell ref) rows are counted exactly.
func TestInspectXLSX_NoDimensionCountsRows(t *testing.T) {
	path := writeXLSX(t, 0, 1, 120, "A1")
	info := inspectXLSXFile(t, path)

	if info.RowCount != 120 {
		t.Errorf("expected 120 data rows, got %d", info.RowCount)
	}
}

// corruptCellRef makes the given cell reference unparsable in the worksheet XML.
func corruptCellRef(t *testing.T, path, cell string) string {
	t.Helper()
	return patchWorksheetXML(t, path, func(data []byte) []byte {
		patched := regexp.MustCompile(`r="`+cell+`"`).ReplaceAll(data, []byte(`r="!!1"`))
		if string(patched) == string(data) {
			t.Fatalf("cell %s not found in worksheet XML", cell)
		}
		return patched
	})
}

// A row that cannot be read must fail the inspection instead of being reported as the end
// of the sheet (which would silently truncate the row count).
func TestInspectXLSX_CorruptRowIsAnError(t *testing.T) {
	path := corruptCellRef(t, writeXLSX(t, 0, 1, 5, ""), "B3")
	if _, err := inspector.New().Inspect(context.Background(), path); err == nil {
		t.Fatal("expected an error for an unreadable row inside the header sample, got nil")
	}
}

// The same applies to rows read past the sample to fill the preview, also when the
// dimension is trusted and no exact count is needed.
func TestInspectXLSX_CorruptRowAfterSampleIsAnError(t *testing.T) {
	// Header on row 36, data rows 37..66: preview rows 6..10 come from rows 42..46.
	path := corruptCellRef(t, writeXLSX(t, 35, 36, 30, "A1:C66"), "B44")
	if _, err := inspector.New().Inspect(context.Background(), path); err == nil {
		t.Fatal("expected an error for an unreadable row past the sample, got nil")
	}
}
