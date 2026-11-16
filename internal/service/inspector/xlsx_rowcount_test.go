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

	dst := filepath.Join(dir, "patched.xlsx")
	zr, err := zip.OpenReader(src)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	zw := zip.NewWriter(out)
	for _, zf := range zr.File {
		rc, err := zf.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if zf.Name == "xl/worksheets/sheet1.xml" {
			if !reSheetDimension.Match(data) {
				t.Fatalf("worksheet has no <dimension> element to patch")
			}
			data = reSheetDimension.ReplaceAll(data, []byte(`<dimension ref="`+dimension+`"/>`))
		}
		w, err := zw.Create(zf.Name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
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

// A row that cannot be read must fail the inspection instead of being reported as the end
// of the sheet (which would silently truncate the row count).
func TestInspectXLSX_CorruptRowIsAnError(t *testing.T) {
	path := writeXLSX(t, 0, 1, 5, "")
	dst := filepath.Join(t.TempDir(), "corrupt.xlsx")
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	for _, zf := range zr.File {
		rc, _ := zf.Open()
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		if zf.Name == "xl/worksheets/sheet1.xml" {
			patched := regexp.MustCompile(`r="B3"`).ReplaceAll(data, []byte(`r="!!3"`))
			if string(patched) == string(data) {
				t.Fatal("cell B3 not found in worksheet XML")
			}
			data = patched
		}
		w, _ := zw.Create(zf.Name)
		_, _ = w.Write(data)
	}
	_ = zw.Close()
	_ = out.Close()

	if _, err := inspector.New().Inspect(context.Background(), dst); err == nil {
		t.Fatal("expected an error for an unreadable row, got nil")
	}
}
