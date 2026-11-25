package inspector_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dep_service/internal/service/inspector"
)

// buildMultiPagePDF writes one content stream per page, one Tj per line.
func buildMultiPagePDF(pages [][]string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")

	n := len(pages)
	fontID := 3 + 2*n
	offsets := make([]int, fontID+1)

	offsets[1] = b.Len()
	b.WriteString("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	kids := make([]string, n)
	for i := range pages {
		kids[i] = fmt.Sprintf("%d 0 R", 3+2*i)
	}
	offsets[2] = b.Len()
	fmt.Fprintf(&b, "2 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n", strings.Join(kids, " "), n)

	for i, lines := range pages {
		pageID, contentID := 3+2*i, 4+2*i
		offsets[pageID] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents %d 0 R /Resources << /Font << /F1 %d 0 R >> >> >>\nendobj\n", pageID, contentID, fontID)

		var cs strings.Builder
		cs.WriteString("BT /F1 11 Tf 40 800 Td")
		for _, l := range lines {
			fmt.Fprintf(&cs, " (%s) Tj 0 -22 Td", l)
		}
		cs.WriteString(" ET")
		offsets[contentID] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", contentID, cs.Len(), cs.String())
	}

	offsets[fontID] = b.Len()
	fmt.Fprintf(&b, "%d 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n", fontID)

	startXref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", fontID+1)
	for i := 1; i <= fontID; i++ {
		fmt.Fprintf(&b, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", fontID+1, startXref)
	return b.Bytes()
}

// Every PII type the PDF preview used to find with its own TypeScript regexes
// must now come back from InspectFile, with the page it is on.
func TestInspectPDF_DetectsAllFormerClientTypesWithPages(t *testing.T) {
	pages := [][]string{
		{
			"Работник: Иванов Иван Иванович, дата рождения 01.02.1985,",
			"паспорт серия 4510 № 123456, СНИЛС 112-233-445 95, ИНН 500100732259.",
			"Телефон: +7 (916) 123-45-67, резервный 8.916.111.22.33, рабочий 7 495 1234567.",
			"Email: ivanov.ivan@example.ru",
			"Проживает: г. Казань, ул. Баумана, д. 17, кв. 3",
		},
		{
			"Работодатель ИНН 7707083893, договор от 15 марта 2024 г.",
			"Директор Сидоров П. А., согласовано: А. В. Кузнецов, 12/05/2023.",
			"Карта 4276 3800 1234 5678, паспорт представителя 45 10 654321.",
			"Уведомили Петрову Анну Сергеевну.",
		},
	}

	pdfPath := filepath.Join(t.TempDir(), "pii.pdf")
	if err := os.WriteFile(pdfPath, buildMultiPagePDF(pages), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := inspector.New().Inspect(context.Background(), pdfPath)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if info.PageCount != 2 {
		t.Errorf("expected 2 pages, got %d", info.PageCount)
	}

	type want struct {
		typ, value string
		page       int
	}
	expected := []want{
		{"FIO", "Иванов Иван Иванович", 1},
		{"DATE", "01.02.1985", 1},
		{"PASSPORT", "4510 № 123456", 1},
		{"SNILS", "112-233-445 95", 1},
		{"INN", "500100732259", 1},
		{"PHONE", "+7 (916) 123-45-67", 1},
		{"PHONE", "8.916.111.22.33", 1},
		{"PHONE", "7 495 1234567", 1},
		{"EMAIL", "ivanov.ivan@example.ru", 1},
		{"ADDRESS", "г. Казань, ул. Баумана, д. 17, кв. 3", 1},
		{"INN", "7707083893", 2},
		{"DATE", "15 марта 2024", 2},
		{"FIO", "Сидоров П. А.", 2},
		{"FIO", "А. В. Кузнецов", 2},
		{"DATE", "12/05/2023", 2},
		{"BANK_CARD", "4276 3800 1234 5678", 2},
		{"PASSPORT", "45 10 654321", 2},
		{"FIO", "Петрову Анну Сергеевну", 2},
	}

	for _, w := range expected {
		found := false
		for _, s := range info.TextSnippets {
			if s.Type == w.typ && s.Original == w.value {
				found = true
				if s.Page != w.page {
					t.Errorf("%s %q: expected page %d, got %d", w.typ, w.value, w.page, s.Page)
				}
				break
			}
		}
		if !found {
			t.Errorf("expected %s %q on page %d; snippets: %+v", w.typ, w.value, w.page, info.TextSnippets)
		}
	}
}

// buildSubsetFontPDF imitates macOS cupsfilter/Quartz output: Cyrillic is shown
// with a TrueType subset whose single-byte codes only a /ToUnicode CMap explains,
// while spaces, digits and punctuation go through a second font, so one word is
// split into several Tj operators with no positioning between them.
func buildSubsetFontPDF(lines []string) []byte {
	codes := map[rune]byte{}
	var order []rune
	next := byte(0x80)
	for _, l := range lines {
		for _, r := range l {
			if r > 0x7F {
				if _, ok := codes[r]; !ok {
					codes[r] = next
					order = append(order, r)
					next++
				}
			}
		}
	}

	var cs strings.Builder
	y := 800
	for _, l := range lines {
		fmt.Fprintf(&cs, "BT 11 0 0 11 40 %d Tm", y)
		y -= 16
		runes := []rune(l)
		for i := 0; i < len(runes); {
			cyr := runes[i] > 0x7F
			j := i
			for j < len(runes) && (runes[j] > 0x7F) == cyr {
				j++
			}
			var lit []byte
			for _, r := range runes[i:j] {
				if cyr {
					lit = append(lit, codes[r])
				} else {
					if r == '(' || r == ')' || r == '\\' {
						lit = append(lit, '\\')
					}
					lit = append(lit, byte(r))
				}
			}
			font := "/F1"
			if cyr {
				font = "/F2"
			}
			fmt.Fprintf(&cs, " %s 1 Tf (%s) Tj", font, lit)
			i = j
		}
		cs.WriteString(" ET\n")
	}

	var cmap strings.Builder
	cmap.WriteString("/CIDInit /ProcSet findresource begin 12 dict begin begincmap\n1 begincodespacerange\n<00> <FF>\nendcodespacerange\n")
	fmt.Fprintf(&cmap, "%d beginbfchar\n", len(order))
	for _, r := range order {
		fmt.Fprintf(&cmap, "<%02X> <%04X>\n", codes[r], r)
	}
	cmap.WriteString("endbfchar\nendcmap\nend end\n")

	objs := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Contents 4 0 R /Resources << /Font << /F1 5 0 R /F2 6 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", cs.Len(), cs.String()),
		"<< /Type /Font /Subtype /TrueType /BaseFont /AAAAAB+Monaco /Encoding /MacRomanEncoding >>",
		"<< /Type /Font /Subtype /TrueType /BaseFont /AAAAAC+Monaco /FirstChar 128 /LastChar 255 /ToUnicode 7 0 R >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", cmap.Len(), cmap.String()),
	}

	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")
	offsets := make([]int, len(objs)+1)
	for i, o := range objs {
		offsets[i+1] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	startXref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for i := 1; i <= len(objs); i++ {
		fmt.Fprintf(&b, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, startXref)
	return b.Bytes()
}

func TestInspectPDF_SubsetFontWithToUnicode(t *testing.T) {
	lines := []string{
		"г. Санкт-Петербург, 12 октября 2026 года",
		"Я, Морозова Елена Сергеевна, паспорт серия 4014 номер 382910,",
		"зарегистрированная по месту жительства по адресу: 197374, г. Санкт-Петербург, ул. Савушкина, д. 126, кв. 48,",
		"даю согласие ООО (адрес: 197374, г. Санкт-Петербург, ул. Савушкина, д. 126, лит. Б, оф. 302) на обработку.",
	}
	pdfPath := filepath.Join(t.TempDir(), "subset.pdf")
	if err := os.WriteFile(pdfPath, buildSubsetFontPDF(lines), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := inspector.New().Inspect(context.Background(), pdfPath)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !strings.Contains(info.PDFTextSnippet, "г. Санкт-Петербург") {
		t.Errorf("Cyrillic must decode through ToUnicode and split Tj must not add spaces, got %q", info.PDFTextSnippet)
	}
	if strings.Contains(info.PDFTextSnippet, "begincmap") || strings.Contains(info.PDFTextSnippet, "041C") {
		t.Errorf("CMap stream leaked into page text: %q", info.PDFTextSnippet)
	}

	expected := map[string]string{
		"Морозова Елена Сергеевна": "FIO",
		"4014 номер 382910":        "PASSPORT",
		"12 октября 2026":          "DATE",
		"197374, г. Санкт-Петербург, ул. Савушкина, д. 126, кв. 48":          "ADDRESS",
		"197374, г. Санкт-Петербург, ул. Савушкина, д. 126, лит. Б, оф. 302": "ADDRESS",
	}
	for value, typ := range expected {
		found := false
		for _, s := range info.TextSnippets {
			if s.Original == value && s.Type == typ {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %s %q; snippets: %+v", typ, value, info.TextSnippets)
		}
	}
}
