package formats

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// rewriteZipEntries copies the xlsx at src to dst, applying edit to the named zip entries.
func rewriteZipEntries(t *testing.T, src, dst string, edits map[string]func(string) string) {
	t.Helper()
	zr, err := zip.OpenReader(src)
	if err != nil {
		t.Fatalf("open source zip: %v", err)
	}
	defer zr.Close()

	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("create destination zip: %v", err)
	}
	defer out.Close()
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
		if edit, ok := edits[zf.Name]; ok {
			data = []byte(edit(string(data)))
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
		t.Fatalf("close destination zip: %v", err)
	}
}

func saveMergedWorkbook(t *testing.T, path string, merges [][2]string) {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	_ = f.SetCellValue("Sheet1", "A1", "Заголовок")
	for _, m := range merges {
		if err := f.MergeCell("Sheet1", m[0], m[1]); err != nil {
			t.Fatalf("merge %s:%s: %v", m[0], m[1], err)
		}
	}
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save workbook: %v", err)
	}
}

// Many merges make the <mergeCells> block span several 64 KB read chunks;
// no reference may be lost on a chunk boundary.
func TestFastExtractMergeRefs_ManyMergesAcrossReadChunks(t *testing.T) {
	const n = 20000
	merges := make([][2]string, 0, n)
	for i := 1; i <= n; i++ {
		merges = append(merges, [2]string{fmt.Sprintf("A%d", i), fmt.Sprintf("C%d", i)})
	}
	p := filepath.Join(t.TempDir(), "many_merges.xlsx")
	saveMergedWorkbook(t, p, merges)

	refs, err := streamExtractMergeRefsFromZip(p, "Sheet1")
	if err != nil {
		t.Fatalf("stream extraction failed: %v", err)
	}
	if len(refs) != n {
		t.Fatalf("expected %d merge refs, got %d", n, len(refs))
	}
	seen := make(map[string]bool, n)
	for _, r := range refs {
		seen[r] = true
	}
	for i := 1; i <= n; i++ {
		if want := fmt.Sprintf("A%d:C%d", i, i); !seen[want] {
			t.Fatalf("merge %s is missing", want)
		}
	}
}

// Workbooks written by .NET OpenXML SDK may use a namespace prefix: <x:mergeCell ref="..."/>.
func TestFastExtractMergeRefs_NamespacePrefixedWorksheet(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.xlsx")
	saveMergedWorkbook(t, src, [][2]string{{"A1", "C1"}, {"A2", "B3"}})

	dst := filepath.Join(dir, "prefixed.xlsx")
	rewriteZipEntries(t, src, dst, map[string]func(string) string{
		"xl/worksheets/sheet1.xml": func(s string) string {
			// Declare the prefix on the root element, as OpenXML SDK does.
			s = strings.Replace(s, "<worksheet ", `<worksheet xmlns:x="http://schemas.openxmlformats.org/spreadsheetml/2006/main" `, 1)
			s = strings.ReplaceAll(s, "<mergeCells", "<x:mergeCells")
			s = strings.ReplaceAll(s, "</mergeCells", "</x:mergeCells")
			s = strings.ReplaceAll(s, "<mergeCell ", "<x:mergeCell ")
			s = strings.ReplaceAll(s, "</mergeCell>", "</x:mergeCell>")
			return s
		},
	})

	refs, err := streamExtractMergeRefsFromZip(dst, "Sheet1")
	if err != nil {
		t.Fatalf("stream extraction failed: %v", err)
	}
	if strings.Join(refs, ",") != "A1:C1,A2:B3" {
		t.Fatalf("expected [A1:C1 A2:B3], got %v", refs)
	}
}

// The word "mergeCells" inside cell text must not be mistaken for the XML element.
func TestFastExtractMergeRefs_IgnoresTextOccurrences(t *testing.T) {
	p := filepath.Join(t.TempDir(), "text.xlsx")
	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "mergeCells <mergeCell ref=\"Z1:Z9\"/>")
	_ = f.SetCellValue("Sheet1", "B1", "x")
	if err := f.MergeCell("Sheet1", "B1", "D1"); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(p); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	refs, err := streamExtractMergeRefsFromZip(p, "Sheet1")
	if err != nil {
		t.Fatalf("stream extraction failed: %v", err)
	}
	if strings.Join(refs, ",") != "B1:D1" {
		t.Fatalf("expected [B1:D1], got %v", refs)
	}
}

func TestFastExtractMergeRefs_NoMerges(t *testing.T) {
	p := filepath.Join(t.TempDir(), "plain.xlsx")
	saveMergedWorkbook(t, p, nil)

	refs, err := streamExtractMergeRefsFromZip(p, "Sheet1")
	if err != nil {
		t.Fatalf("stream extraction failed: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("expected no merges, got %v", refs)
	}
}

// Relationship targets may be absolute part names ("/xl/worksheets/sheet1.xml").
func TestFastExtractMergeRefs_AbsoluteRelationshipTarget(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.xlsx")
	saveMergedWorkbook(t, src, [][2]string{{"A1", "B2"}})

	dst := filepath.Join(dir, "absolute.xlsx")
	rewriteZipEntries(t, src, dst, map[string]func(string) string{
		"xl/_rels/workbook.xml.rels": func(s string) string {
			return strings.Replace(s, `Target="worksheets/sheet1.xml"`, `Target="/xl/worksheets/sheet1.xml"`, 1)
		},
	})

	zr, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatal(err)
	}
	got := resolveSheetXMLPath(zr, "Sheet1")
	_ = zr.Close()
	if got != "xl/worksheets/sheet1.xml" {
		t.Fatalf("expected xl/worksheets/sheet1.xml, got %q", got)
	}

	refs, err := streamExtractMergeRefsFromZip(dst, "Sheet1")
	if err != nil {
		t.Fatalf("stream extraction failed: %v", err)
	}
	if strings.Join(refs, ",") != "A1:B2" {
		t.Fatalf("expected [A1:B2], got %v", refs)
	}
}

// Markup-like text inside CDATA sections and XML comments must not be taken for the real
// <mergeCells> block, including sections larger than one read chunk.
func TestFastExtractMergeRefs_SkipsCommentsAndCDATA(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.xlsx")
	saveMergedWorkbook(t, src, [][2]string{{"A1", "C1"}})

	fake := `<mergeCells count="1"><mergeCell ref="Z1:Z9"/></mergeCells>`
	padding := strings.Repeat("a", 150*1024)
	dst := filepath.Join(dir, "cdata.xlsx")
	rewriteZipEntries(t, src, dst, map[string]func(string) string{
		"xl/worksheets/sheet1.xml": func(s string) string {
			s = strings.Replace(s, "<sheetData>", "<!-- "+fake+" --><sheetData>", 1)
			cell := `<row r="50"><c r="A50" t="inlineStr"><is><t><![CDATA[` + padding + fake + `]]></t></is></c></row>`
			return strings.Replace(s, "</sheetData>", cell+"</sheetData>", 1)
		},
	})

	refs, err := streamExtractMergeRefsFromZip(dst, "Sheet1")
	if err != nil {
		t.Fatalf("stream extraction failed: %v", err)
	}
	if strings.Join(refs, ",") != "A1:C1" {
		t.Fatalf("expected only the real merge [A1:C1], got %v", refs)
	}
}

// Feeding the scanner one byte at a time splits every opener, terminator and tag name.
func TestXMLStartTagScanner_ByteByByte(t *testing.T) {
	doc := `<worksheet><sheetData><row><c><is><t><![CDATA[<mergeCells>]]></t></is></c></row></sheetData>` +
		`<!-- <mergeCells> --><!DOCTYPE x><mergeCellsX/><x:mergeCells count="1"><x:mergeCell ref="A1:B1"/></x:mergeCells></worksheet>`
	want := strings.Index(doc, "<x:mergeCells ")

	s := xmlStartTagScanner{local: mergeCellsTag}
	consumed := 0
	for i := 0; i < len(doc); i++ {
		if pos := s.feed([]byte{doc[i]}); pos >= 0 {
			got := consumed + 1 - len(s.pending) + pos
			if got != want {
				t.Fatalf("found tag at offset %d, want %d", got, want)
			}
			return
		}
		consumed++
	}
	t.Fatal("real <x:mergeCells> start tag was not found")
}
