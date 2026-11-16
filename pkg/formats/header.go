package formats

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"unicode"

	"github.com/xuri/excelize/v2"
)

// ExcelMergeRange represents a merged cell range and its string value for hierarchical header resolution.
type ExcelMergeRange struct {
	StartCol int
	StartRow int
	EndCol   int
	EndRow   int
	Value    string
}

// HeaderBounds represents the vertical boundaries of a tabular dataset's header structure.
type HeaderBounds struct {
	HeaderStartIdx int // 0-based index of the top-most category or header row
	LeafHeaderIdx  int // 0-based index of the bottom-most leaf header row
	DataStartIdx   int // 0-based index of the first actual data row
}

// isColumnNumberingRow detects sequential integer column numbering (e.g. 1, 2, 3... 27)
// commonly generated in Russian GOST 2.105, 1C:Enterprise, SAP Russian localization, and ERP exports.
func isColumnNumberingRow(row []string) bool {
	if len(row) < 3 {
		return false
	}
	sequential := 0
	nonEmpty := 0
	for i, c := range row {
		trimmed := strings.TrimSpace(c)
		if trimmed == "" {
			continue
		}
		nonEmpty++
		if n, err := strconv.Atoi(trimmed); err == nil {
			if n == i+1 || n == nonEmpty {
				sequential++
			}
		}
	}
	return nonEmpty >= 3 && float64(sequential)/float64(nonEmpty) >= 0.8
}

// DetectHeaderBounds accurately locates the start, leaf header, and data start rows
// across simple tables, multi-tier hierarchical headers with vertical/horizontal merges,
// and tables containing sequential column numbering rows (GOST / 1C / SAP standard).
func DetectHeaderBounds(rows [][]string, merges []ExcelMergeRange) HeaderBounds {
	if len(rows) == 0 {
		return HeaderBounds{HeaderStartIdx: 0, LeafHeaderIdx: 0, DataStartIdx: 0}
	}
	if len(rows) == 1 {
		return HeaderBounds{HeaderStartIdx: 0, LeafHeaderIdx: 0, DataStartIdx: 1}
	}

	pivotIdx, _ := FindHeaderRow(rows)
	if pivotIdx < 0 || pivotIdx >= len(rows) {
		pivotIdx = 0
	}

	// Downward scan from pivotIdx: check if rows below pivotIdx are continuation of multi-tier header.
	// In spreadsheet formats, a multi-tier header row is ALWAYS governed by merged cells:
	// either vertical merges spanning across the row from above, or sub-headers directly beneath
	// horizontal category merges from the row above.
	leafIdx := pivotIdx
	limit := len(rows)
	if limit > pivotIdx+10 {
		limit = pivotIdx + 10
	}

	for r := leafIdx + 1; r < limit; r++ {
		// Stop immediately if row is an obvious column numbering row (1, 2, 3...)
		if isColumnNumberingRow(rows[r]) {
			break
		}

		r1Based := r + 1
		hasVertMerge := false
		for _, m := range merges {
			if m.StartRow <= pivotIdx+1 && m.EndRow >= r1Based && m.EndRow > m.StartRow {
				hasVertMerge = true
				break
			}
		}

		// Check if the row immediately above had horizontal category merges
		hasHMergeAbove := false
		prevR1Based := r // 1-based for r-1
		for _, m := range merges {
			if m.StartRow <= prevR1Based && prevR1Based <= m.EndRow && m.EndCol > m.StartCol {
				hasHMergeAbove = true
				break
			}
		}

		// Downward expansion REQUIRES a merge relation (vertical merge or horizontal category above)
		if !hasVertMerge && !hasHMergeAbove {
			break
		}

		// Verify row r is NOT an obvious data row (e.g. contains mock names, emails, or pure data values)
		isDataRow := false
		nonEmpty := 0
		hasText := false
		for _, c := range rows[r] {
			v := strings.TrimSpace(c)
			if v == "" {
				continue
			}
			nonEmpty++
			if reFIOPattern.MatchString(v) || reMockFIOPattern.MatchString(v) || reEmailPattern.MatchString(v) {
				isDataRow = true
				break
			}
			if _, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64); err == nil {
				// numeric cell
			} else {
				hasText = true
			}
		}

		if isDataRow || nonEmpty == 0 || (!hasVertMerge && !hasText) {
			break
		}

		leafIdx = r
	}

	// Upward scan from leafIdx to find headerStartIdx
	numCols := len(rows[leafIdx])
	for r := 0; r <= leafIdx; r++ {
		if len(rows[r]) > numCols {
			numCols = len(rows[r])
		}
	}
	for _, m := range merges {
		if m.EndCol > numCols && m.StartRow <= leafIdx+2 {
			numCols = m.EndCol
		}
	}

	headerStartIdx := leafIdx
	for r := leafIdx - 1; r >= 0; r-- {
		if countNonEmpty(rows[r]) == 0 {
			break
		}

		hasCategoryMerge := false
		r1Based := r + 1
		for _, m := range merges {
			if m.StartRow <= r1Based && r1Based <= m.EndRow && m.EndCol > m.StartCol && m.Value != "" {
				span := m.EndCol - m.StartCol + 1
				if numCols >= 4 && span >= int(float64(numCols)*0.8) {
					continue
				}
				hasCategoryMerge = true
				break
			}
		}

		nonEmpty := countNonEmpty(rows[r])
		if hasCategoryMerge || (nonEmpty >= 2 && !isPreambleRow(rows[r])) {
			headerStartIdx = r
		} else {
			break
		}
	}

	// Determine dataStartIdx: check if the row immediately following leafIdx is a column numbering row
	dataStartIdx := leafIdx + 1
	if dataStartIdx < len(rows) && isColumnNumberingRow(rows[dataStartIdx]) {
		dataStartIdx++
	}

	return HeaderBounds{
		HeaderStartIdx: headerStartIdx,
		LeafHeaderIdx:  leafIdx,
		DataStartIdx:   dataStartIdx,
	}
}

type openXMLWorkbook struct {
	Sheets []struct {
		Name string `xml:"name,attr"`
		RID  string `xml:"id,attr"`
	} `xml:"sheets>sheet"`
}

type openXMLRels struct {
	Relationships []struct {
		ID     string `xml:"Id,attr"`
		Target string `xml:"Target,attr"`
	} `xml:"Relationship"`
}

func resolveSheetXMLPath(zr *zip.ReadCloser, sheetName string) string {
	var wbFile, relsFile *zip.File
	for _, f := range zr.File {
		clean := strings.ToLower(strings.ReplaceAll(f.Name, "\\", "/"))
		if clean == "xl/workbook.xml" {
			wbFile = f
		} else if clean == "xl/_rels/workbook.xml.rels" {
			relsFile = f
		}
	}
	if wbFile == nil {
		return ""
	}

	wbRC, err := wbFile.Open()
	if err != nil {
		return ""
	}
	defer wbRC.Close()

	var wb openXMLWorkbook
	if err := xml.NewDecoder(wbRC).Decode(&wb); err != nil {
		return ""
	}

	targetRID := ""
	for _, s := range wb.Sheets {
		if strings.EqualFold(s.Name, sheetName) {
			targetRID = s.RID
			break
		}
	}
	if targetRID == "" {
		return ""
	}

	if relsFile == nil {
		return ""
	}
	relsRC, err := relsFile.Open()
	if err != nil {
		return ""
	}
	defer relsRC.Close()

	var rels openXMLRels
	if err := xml.NewDecoder(relsRC).Decode(&rels); err != nil {
		return ""
	}

	for _, r := range rels.Relationships {
		if r.ID == targetRID {
			target := strings.ReplaceAll(r.Target, "\\", "/")
			if strings.HasPrefix(target, "/") {
				// Absolute part name relative to the package root, e.g. "/xl/worksheets/sheet1.xml".
				return strings.TrimPrefix(path.Clean(target), "/")
			}
			// Relative to the workbook part directory "xl/".
			return path.Join("xl", target)
		}
	}
	return ""
}

func streamExtractMergeRefsFromZip(zipPath, sheetName string) ([]string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	sheetTarget := resolveSheetXMLPath(zr, sheetName)
	if sheetTarget == "" {
		return nil, fmt.Errorf("sheet %s not found in workbook", sheetName)
	}

	var sheetFile *zip.File
	for _, f := range zr.File {
		cleanName := strings.ReplaceAll(f.Name, "\\", "/")
		if strings.EqualFold(cleanName, sheetTarget) || strings.HasSuffix(strings.ToLower(cleanName), "/"+strings.ToLower(sheetTarget)) {
			sheetFile = f
			break
		}
	}
	if sheetFile == nil {
		return nil, fmt.Errorf("sheet XML file %s not found in zip", sheetTarget)
	}

	rc, err := sheetFile.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	// The bulky <sheetData> section is skipped with a fast byte scan for the start of the
	// <mergeCells> container (optionally namespace-prefixed, e.g. <x:mergeCells>). Only the
	// container itself is handed to a real XML decoder, so references are never lost on
	// read-chunk boundaries and attribute quoting/order does not matter.
	buf := make([]byte, 64*1024)
	scanner := xmlStartTagScanner{local: mergeCellsTag}

	for {
		n, readErr := rc.Read(buf)
		if n > 0 {
			if pos := scanner.feed(buf[:n]); pos >= 0 {
				dec := xml.NewDecoder(io.MultiReader(bytes.NewReader(scanner.pending[pos:]), rc))
				return decodeMergeCells(dec)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				// No <mergeCells> element: the sheet has no merged cells.
				return nil, nil
			}
			return nil, readErr
		}
	}
}

var (
	mergeCellsTag      = []byte("mergeCells")
	xmlMarkupDeclStart = []byte("<!")
	xmlCommentStart    = []byte("<!--")
	xmlCommentEnd      = []byte("-->")
	xmlCDATAStart      = []byte("<![CDATA[")
	xmlCDATAEnd        = []byte("]]>")
)

// xmlStartTagScannerTail is enough to hold "<" + namespace prefix + local name across reads.
const xmlStartTagScannerTail = 256

// xmlStartTagScanner finds the start tag of an element in a byte stream without parsing the
// XML in between. Comments and CDATA sections are skipped, so markup-like text inside them
// (e.g. a cell containing "<![CDATA[<mergeCells>...]]>") is never mistaken for the element.
type xmlStartTagScanner struct {
	local   []byte
	pending []byte // unscanned bytes; feed results are offsets into it
	skipEnd []byte // terminator of the comment/CDATA section being skipped, nil otherwise
}

// feed appends data and returns the offset of the tag's '<' in s.pending, or -1 if more
// input is needed.
func (s *xmlStartTagScanner) feed(data []byte) int {
	s.pending = append(s.pending, data...)
	for {
		if s.skipEnd != nil {
			i := bytes.Index(s.pending, s.skipEnd)
			if i < 0 {
				s.keepTail(len(s.skipEnd) - 1)
				return -1
			}
			s.pending = s.pending[i+len(s.skipEnd):]
			s.skipEnd = nil
			continue
		}

		pos := findXMLStartTag(s.pending, s.local)
		bang := bytes.Index(s.pending, xmlMarkupDeclStart)
		if bang < 0 || (pos >= 0 && pos < bang) {
			if pos >= 0 {
				return pos
			}
			s.keepTail(xmlStartTagScannerTail)
			return -1
		}

		rest := s.pending[bang:]
		switch {
		case bytes.HasPrefix(rest, xmlCommentStart):
			s.pending, s.skipEnd = rest[len(xmlCommentStart):], xmlCommentEnd
		case bytes.HasPrefix(rest, xmlCDATAStart):
			s.pending, s.skipEnd = rest[len(xmlCDATAStart):], xmlCDATAEnd
		case bytes.HasPrefix(xmlCommentStart, rest) || bytes.HasPrefix(xmlCDATAStart, rest):
			// The opener is split across reads: wait for more input.
			s.pending = rest
			return -1
		default:
			// Another declaration (e.g. <!DOCTYPE>): nothing to skip.
			s.pending = rest[len(xmlMarkupDeclStart):]
		}
	}
}

func (s *xmlStartTagScanner) keepTail(n int) {
	if len(s.pending) > n {
		s.pending = append(s.pending[:0], s.pending[len(s.pending)-n:]...)
	}
}

// findXMLStartTag returns the offset of the '<' that opens a start tag with the given local
// name (with or without a namespace prefix), or -1 if no complete tag name is present in data.
func findXMLStartTag(data, local []byte) int {
	off := 0
	for {
		i := bytes.Index(data[off:], local)
		if i < 0 {
			return -1
		}
		i += off
		end := i + len(local)
		if end >= len(data) {
			// The byte after the name is not read yet; the caller keeps the tail and retries.
			return -1
		}
		if isXMLNameTerminator(data[end]) {
			if tagStart := xmlTagStartBefore(data, i); tagStart >= 0 {
				return tagStart
			}
		}
		off = i + 1
	}
}

// xmlTagStartBefore checks that the name at data[nameIdx:] directly follows "<" or "<prefix:"
// and returns the offset of '<', or -1 otherwise (e.g. the word occurs inside cell text).
func xmlTagStartBefore(data []byte, nameIdx int) int {
	j := nameIdx - 1
	if j >= 0 && data[j] == ':' {
		j--
		prefixEnd := j
		for j >= 0 && isXMLNameByte(data[j]) {
			j--
		}
		if j == prefixEnd {
			return -1 // empty prefix
		}
	}
	if j >= 0 && data[j] == '<' {
		return j
	}
	return -1
}

func isXMLNameByte(b byte) bool {
	return b == '_' || b == '-' || b == '.' || b >= 0x80 ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func isXMLNameTerminator(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n' || b == '>' || b == '/'
}

// decodeMergeCells reads <mergeCell ref="..."/> elements until the closing </mergeCells>.
func decodeMergeCells(dec *xml.Decoder) ([]string, error) {
	var refs []string
	for {
		// RawToken: no namespace resolution is needed (only local names are compared),
		// and the section may use prefixes declared on the root element we skipped.
		tok, err := dec.RawToken()
		if err != nil {
			return nil, fmt.Errorf("failed to parse mergeCells: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local != "mergeCell" {
				continue
			}
			for _, a := range t.Attr {
				if a.Name.Local == "ref" && a.Value != "" {
					refs = append(refs, a.Value)
					break
				}
			}
		case xml.EndElement:
			if t.Name.Local == "mergeCells" {
				return refs, nil
			}
		}
	}
}

// FastExtractMergeRefs extracts merge cell reference strings (e.g. "A1:B2") without loading the entire worksheet DOM into memory.
// It opens the zip archive on disk and streams the XML for the specified sheet.
// If zipPath is empty or the streaming fails, it falls back to xl.GetMergeCells.
func FastExtractMergeRefs(xl *excelize.File, zipPath, sheetName string) []string {
	if zipPath == "" && xl != nil {
		zipPath = xl.Path
	}
	if zipPath != "" {
		if refs, err := streamExtractMergeRefsFromZip(zipPath, sheetName); err == nil {
			return refs
		}
	}
	if xl != nil {
		if rawMerges, err := xl.GetMergeCells(sheetName, true); err == nil {
			refs := make([]string, 0, len(rawMerges))
			for _, m := range rawMerges {
				start := m.GetStartAxis()
				end := m.GetEndAxis()
				if start != "" && end != "" {
					refs = append(refs, start+":"+end)
				}
			}
			return refs
		}
	}
	return nil
}

// ExtractMergeRanges extracts merge cell coordinates and their top-left values from an Excel worksheet.
// If sampleRows is provided, cell values for header merges are resolved directly from the sample
// without triggering full DOM loading or cell queries on the entire workbook.
func ExtractMergeRanges(xl *excelize.File, sheetName string, sampleRows ...[][]string) []ExcelMergeRange {
	if xl == nil {
		return nil
	}
	var sample [][]string
	if len(sampleRows) > 0 {
		sample = sampleRows[0]
	}

	filePath := xl.Path
	rawRefs := FastExtractMergeRefs(xl, filePath, sheetName)
	if len(rawRefs) == 0 {
		return nil
	}

	ranges := make([]ExcelMergeRange, 0, len(rawRefs))
	for _, ref := range rawRefs {
		parts := strings.Split(ref, ":")
		if len(parts) != 2 {
			continue
		}
		sCol, sRow, err1 := excelize.CellNameToCoordinates(parts[0])
		eCol, eRow, err2 := excelize.CellNameToCoordinates(parts[1])
		if err1 != nil || err2 != nil {
			continue
		}

		val := ""
		sRow0 := sRow - 1
		sCol0 := sCol - 1
		if sRow0 >= 0 && sRow0 < len(sample) && sCol0 >= 0 && sCol0 < len(sample[sRow0]) {
			val = sample[sRow0][sCol0]
		} else if sRow <= 40 && len(sample) == 0 {
			// Only query cell value if sample wasn't provided and row is in header range
			val, _ = xl.GetCellValue(sheetName, parts[0])
		}

		ranges = append(ranges, ExcelMergeRange{
			StartCol: sCol,
			StartRow: sRow,
			EndCol:   eCol,
			EndRow:   eRow,
			Value:    strings.TrimSpace(val),
		})
	}
	return ranges
}

// BuildHierarchicalHeaders constructs unified column headers for multi-level/hierarchical
// table headers (spanning headerStartRow to headerEndRow), taking into account merged cells
// or multi-tier category rows above the primary leaf header row.
func BuildHierarchicalHeaders(rows [][]string, leafHeaderIdx int, merges []ExcelMergeRange) []string {
	if len(rows) == 0 {
		return nil
	}
	if leafHeaderIdx < 0 || leafHeaderIdx >= len(rows) {
		leafHeaderIdx = 0
	}

	leafRow := rows[leafHeaderIdx]
	numCols := len(leafRow)
	for r := 0; r <= leafHeaderIdx; r++ {
		if len(rows[r]) > numCols {
			numCols = len(rows[r])
		}
	}
	for _, m := range merges {
		if m.EndCol > numCols && m.StartRow <= leafHeaderIdx+2 {
			numCols = m.EndCol
		}
	}
	if leafHeaderIdx+1 < len(rows) && len(rows[leafHeaderIdx+1]) > numCols {
		numCols = len(rows[leafHeaderIdx+1])
	}
	if leafHeaderIdx+2 < len(rows) && len(rows[leafHeaderIdx+2]) > numCols {
		numCols = len(rows[leafHeaderIdx+2])
	}
	if numCols == 0 {
		return nil
	}

	// If leafHeaderIdx is 0 and no merges exist, return standard row 0 headers
	if leafHeaderIdx == 0 && len(merges) == 0 {
		res := make([]string, len(leafRow))
		copy(res, leafRow)
		return res
	}

	// Determine headerStartIdx by scanning upward from leafHeaderIdx - 1
	headerStartIdx := leafHeaderIdx
	for r := leafHeaderIdx - 1; r >= 0; r-- {
		// If row is completely empty, stop (blank row separates metadata from header)
		if countNonEmpty(rows[r]) == 0 {
			break
		}

		// Check if row r contains category merges that group columns (not a full-width title banner)
		hasCategoryMerge := false
		r1Based := r + 1
		for _, m := range merges {
			if m.StartRow <= r1Based && r1Based <= m.EndRow && m.EndCol > m.StartCol && m.Value != "" {
				// If the merge spans the entire or almost entire table (>= 80% cols and numCols >= 4), it's a report title banner
				span := m.EndCol - m.StartCol + 1
				if numCols >= 4 && span >= int(float64(numCols)*0.8) {
					continue
				}
				hasCategoryMerge = true
				break
			}
		}

		// A row is part of header block if it has column category merges,
		// or if it has multiple non-empty cells that don't look like document preamble titles
		nonEmpty := countNonEmpty(rows[r])
		if hasCategoryMerge || (nonEmpty >= 2 && !isPreambleRow(rows[r])) {
			headerStartIdx = r
		} else {
			// Single non-empty cell or preamble title row terminates header scanning upward
			break
		}
	}

	// Build a coordinate lookup for merges: key "col:row" (1-based)
	// Intersect merge ranges with header bounds [headerStartIdx+1, leafHeaderIdx+1] and [1, numCols]
	// to avoid materializing huge non-header ranges in memory.
	mergeMap := make(map[string]string)
	minHRow1 := headerStartIdx + 1
	maxHRow1 := leafHeaderIdx + 1
	for _, m := range merges {
		if m.Value == "" {
			continue
		}
		startR := m.StartRow
		if startR < minHRow1 {
			startR = minHRow1
		}
		endR := m.EndRow
		if endR > maxHRow1 {
			endR = maxHRow1
		}
		startC := m.StartCol
		if startC < 1 {
			startC = 1
		}
		endC := m.EndCol
		if endC > numCols {
			endC = numCols
		}
		if startR > endR || startC > endC {
			continue
		}

		for r := startR; r <= endR; r++ {
			for c := startC; c <= endC; c++ {
				key := fmt.Sprintf("%d:%d", c, r)
				mergeMap[key] = m.Value
			}
		}
	}

	combined := make([]string, numCols)
	for c := 0; c < numCols; c++ {
		c1Based := c + 1
		var parts []string
		for r := headerStartIdx; r <= leafHeaderIdx; r++ {
			r1Based := r + 1
			key := fmt.Sprintf("%d:%d", c1Based, r1Based)
			val := ""
			if mv, ok := mergeMap[key]; ok && mv != "" {
				val = mv
			} else if r < len(rows) && c < len(rows[r]) {
				val = strings.TrimSpace(rows[r][c])
			}

			if val != "" {
				// Deduplicate consecutive identical segments
				if len(parts) == 0 || parts[len(parts)-1] != val {
					parts = append(parts, val)
				}
			}
		}

		if len(parts) == 0 {
			if c < len(leafRow) && strings.TrimSpace(leafRow[c]) != "" {
				combined[c] = strings.TrimSpace(leafRow[c])
			} else {
				combined[c] = fmt.Sprintf("Колонка %d", c+1)
			}
		} else {
			combined[c] = strings.Join(parts, " / ")
		}
	}

	// Disambiguate identical paths (e.g. repeated leaf names under identical parents)
	seen := make(map[string]int, len(combined))
	for i, name := range combined {
		seen[name]++
		if count := seen[name]; count > 1 {
			combined[i] = fmt.Sprintf("%s (%d)", name, count)
		}
	}

	return combined
}

func isPreambleRow(row []string) bool {
	joined := strings.ToLower(strings.Join(row, " "))
	preambleMarkers := []string{"отчёт", "отчет", "реестр", "ведомость", "выгрузка", "сформирован", "период:", "организация:", "структурное подразделение"}
	for _, m := range preambleMarkers {
		if strings.Contains(joined, m) && countNonEmpty(row) <= 3 {
			return true
		}
	}
	return false
}

// FindHeaderRow examines sampled leading rows from a spreadsheet or tabular dataset
// and identifies the index of the primary table header row (0-based) along with its column headers.
// If the table begins with preamble metadata (report title, filters, dates, empty rows),
// FindHeaderRow skips those preamble rows and locates the row containing actual column headers.
func FindHeaderRow(rows [][]string) (int, []string) {
	if len(rows) == 0 {
		return 0, nil
	}
	if len(rows) == 1 {
		return 0, rows[0]
	}

	bestIdx := 0
	maxScore := -1
	var bestHeaders []string

	limit := len(rows)
	if limit > 40 {
		limit = 40
	}

	for i := 0; i < limit; i++ {
		row := rows[i]
		nonEmpty := 0
		textCells := 0
		for _, cell := range row {
			t := strings.TrimSpace(strings.TrimPrefix(cell, "\ufeff"))
			if t != "" {
				nonEmpty++
				if !isPureNumericOrDate(t) {
					textCells++
				}
			}
		}

		if nonEmpty < 2 {
			continue
		}

		// Look ahead to next non-empty row
		nextNonEmpty := 0
		for nextIdx := i + 1; nextIdx < len(rows) && nextIdx <= i+3; nextIdx++ {
			c := countNonEmpty(rows[nextIdx])
			if c > 0 {
				nextNonEmpty = c
				break
			}
		}

		score := nonEmpty*2 + textCells*3
		if nextNonEmpty >= 2 {
			score += 15
		}

		// Bonus for standard table header keywords
		joined := strings.ToLower(strings.Join(row, " "))
		keywords := []string{
			"фио", "сотрудник", "организация", "должность", "дата", "номер", "почта", "статус",
			"курс", "клиент", "паспорт", "снилс", "инн", "телефон", "карта", "id", "name",
			"код", "наименование", "сумма", "результат", "подразделение",
			"участник", "кандидат", "респондент", "пользователь", "mail", "время",
		}
		for _, kw := range keywords {
			if strings.Contains(joined, kw) {
				score += 6
			}
		}

		if score > maxScore {
			maxScore = score
			bestIdx = i
			bestHeaders = row
		}
	}

	if maxScore < 0 {
		return 0, rows[0]
	}
	return bestIdx, bestHeaders
}

func countNonEmpty(row []string) int {
	c := 0
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			c++
		}
	}
	return c
}

func isPureNumericOrDate(s string) bool {
	digits := 0
	total := 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			digits++
		}
		if !unicode.IsSpace(r) {
			total++
		}
	}
	return total > 0 && float64(digits)/float64(total) > 0.6
}
