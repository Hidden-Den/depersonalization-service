package formats_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/xuri/excelize/v2"

	"dep_service/internal/domain"
	"dep_service/pkg/anonymizer"
	"dep_service/pkg/depmap"
	"dep_service/pkg/formats"
)

func TestRestoreCSV(t *testing.T) {
	dm := depmap.New("clients.csv")
	dm.Put("Смирнов Алексей Петрович", "Иванов Иван Иванович")
	dm.Put("452-190-883 12", "112-233-445 95")
	dm.Put("Кузнецова Ольга Ивановна", "Петрова Анна Сергеевна")
	dm.Put("987-654-321 00", "223-344-556 01")

	inputCSV := "ФИО,СНИЛС,Score_ML,Category\n" +
		"Смирнов Алексей Петрович,452-190-883 12,0.95,VIP\n" +
		"Кузнецова Ольга Ивановна,987-654-321 00,0.42,Standard\n" +
		"Сидоров Петр Алексеевич,000-111-222 33,0.15,New\n"

	var out bytes.Buffer
	ctx := context.Background()

	restoredCount, err := formats.RestoreCSV(ctx, strings.NewReader(inputCSV), &out, dm)
	if err != nil {
		t.Fatalf("RestoreCSV failed: %v", err)
	}

	if restoredCount != 4 {
		t.Errorf("expected 4 restored cells, got %d", restoredCount)
	}

	reader := csv.NewReader(&out)
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read restored csv: %v", err)
	}

	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}

	// Verify headers preserved intact
	expectedHeaders := []string{"ФИО", "СНИЛС", "Score_ML", "Category"}
	for i, h := range expectedHeaders {
		if rows[0][i] != h {
			t.Errorf("header col %d mismatch: expected %q, got %q", i, h, rows[0][i])
		}
	}

	// Row 1: restored FIO and SNILS, preserved Score_ML and Category
	if rows[1][0] != "Иванов Иван Иванович" {
		t.Errorf("row 1 FIO mismatch: expected 'Иванов Иван Иванович', got %q", rows[1][0])
	}
	if rows[1][1] != "112-233-445 95" {
		t.Errorf("row 1 SNILS mismatch: expected '112-233-445 95', got %q", rows[1][1])
	}
	if rows[1][2] != "0.95" {
		t.Errorf("row 1 Score_ML mismatch: expected '0.95', got %q", rows[1][2])
	}
	if rows[1][3] != "VIP" {
		t.Errorf("row 1 Category mismatch: expected 'VIP', got %q", rows[1][3])
	}

	// Row 2: restored FIO and SNILS
	if rows[2][0] != "Петрова Анна Сергеевна" {
		t.Errorf("row 2 FIO mismatch: expected 'Петрова Анна Сергеевна', got %q", rows[2][0])
	}
	if rows[2][1] != "223-344-556 01" {
		t.Errorf("row 2 SNILS mismatch: expected '223-344-556 01', got %q", rows[2][1])
	}
	if rows[2][2] != "0.42" {
		t.Errorf("row 2 Score_ML mismatch: expected '0.42', got %q", rows[2][2])
	}
	if rows[2][3] != "Standard" {
		t.Errorf("row 2 Category mismatch: expected 'Standard', got %q", rows[2][3])
	}

	// Row 3: unmapped values remain unchanged
	if rows[3][0] != "Сидоров Петр Алексеевич" {
		t.Errorf("row 3 FIO unmapped mismatch: expected 'Сидоров Петр Алексеевич', got %q", rows[3][0])
	}
	if rows[3][1] != "000-111-222 33" {
		t.Errorf("row 3 SNILS unmapped mismatch: expected '000-111-222 33', got %q", rows[3][1])
	}
	if rows[3][2] != "0.15" {
		t.Errorf("row 3 Score_ML mismatch: expected '0.15', got %q", rows[3][2])
	}
	if rows[3][3] != "New" {
		t.Errorf("row 3 Category mismatch: expected 'New', got %q", rows[3][3])
	}
}

func TestRestoreCSV_SemicolonAndBOM(t *testing.T) {
	dm := depmap.New("semicolon.csv")
	dm.Put("Синтетика Один", "Оригинал Один")

	// UTF-8 BOM + semicolon-separated CSV
	inputCSV := "\ufeffФИО;Метрика\nСинтетика Один;100\n"

	var out bytes.Buffer
	ctx := context.Background()

	restoredCount, err := formats.RestoreCSV(ctx, strings.NewReader(inputCSV), &out, dm)
	if err != nil {
		t.Fatalf("RestoreCSV failed: %v", err)
	}

	if restoredCount != 1 {
		t.Errorf("expected 1 restored cell, got %d", restoredCount)
	}

	reader := csv.NewReader(&out)
	reader.Comma = ';'
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read restored semicolon csv: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[1][0] != "Оригинал Один" {
		t.Errorf("expected 'Оригинал Один', got %q", rows[1][0])
	}
	if rows[1][1] != "100" {
		t.Errorf("expected '100', got %q", rows[1][1])
	}
}

func TestRestoreExcel(t *testing.T) {
	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, "scored_pipeline.xlsx")
	outputPath := filepath.Join(tmpDir, "restored_pipeline.xlsx")

	dm := depmap.New("original.xlsx")
	dm.Put("Васильев Василий Васильевич", "Иванов Иван Иванович")
	dm.Put("452-190-883 12", "112-233-445 95")
	dm.Put("+7 (999) 000-11-22", "+7 (916) 123-45-67")

	// Create test workbook with 2 sheets
	f := excelize.NewFile()
	defer f.Close()

	sheet1 := "Clients"
	f.SetSheetName("Sheet1", sheet1)

	// Sheet 1: has synthetic FIO and SNILS, plus ML pipeline added columns: Score, Segment, Status
	headers1 := []string{"ФИО", "СНИЛС", "ML_Score", "Segment", "Model_Status"}
	for i, h := range headers1 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet1, cell, h)
	}

	row1 := []interface{}{"Васильев Василий Васильевич", "452-190-883 12", 0.88, "A", "APPROVED"}
	for i, v := range row1 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(sheet1, cell, v)
	}

	row2 := []interface{}{"Неизвестный Кандидат", "111-222-333 44", 0.12, "C", "REJECTED"}
	for i, v := range row2 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 3)
		_ = f.SetCellValue(sheet1, cell, v)
	}

	// Sheet 2: "Contacts"
	sheet2 := "Contacts"
	_, _ = f.NewSheet(sheet2)
	headers2 := []string{"Телефон", "Email_Verified", "Priority"}
	for i, h := range headers2 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet2, cell, h)
	}

	sheet2Row := []interface{}{"+7 (999) 000-11-22", true, 10}
	for i, v := range sheet2Row {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(sheet2, cell, v)
	}

	if err := f.SaveAs(inputPath); err != nil {
		t.Fatalf("failed to save initial xlsx: %v", err)
	}

	var progressCalled int32
	var lastPercent int
	progressFn := func(pct int, proc, total int) {
		atomic.AddInt32(&progressCalled, 1)
		lastPercent = pct
		if proc > total && total > 0 {
			t.Errorf("progress proc (%d) > total (%d)", proc, total)
		}
	}

	ctx := context.Background()
	restoredCount, err := formats.RestoreExcel(ctx, inputPath, outputPath, dm, progressFn)
	if err != nil {
		t.Fatalf("RestoreExcel failed: %v", err)
	}

	// Expect 3 restored cells: 1 FIO, 1 SNILS on Sheet1, 1 Phone on Sheet2
	if restoredCount != 3 {
		t.Errorf("expected 3 restored cells, got %d", restoredCount)
	}

	if atomic.LoadInt32(&progressCalled) == 0 {
		t.Errorf("expected progress callback to be invoked at least once")
	}
	if lastPercent != 100 {
		t.Errorf("expected final progress percentage to be 100, got %d", lastPercent)
	}

	// Read and verify restored workbook
	rf, err := excelize.OpenFile(outputPath)
	if err != nil {
		t.Fatalf("failed to open restored xlsx file: %v", err)
	}
	defer rf.Close()

	// Verify Sheet 1
	rows1, err := rf.GetRows(sheet1)
	if err != nil {
		t.Fatalf("failed to get rows for %s: %v", sheet1, err)
	}
	if len(rows1) < 3 {
		t.Fatalf("expected at least 3 rows in %s, got %d", sheet1, len(rows1))
	}

	// Header check
	for i, h := range headers1 {
		if rows1[0][i] != h {
			t.Errorf("sheet1 header col %d mismatch: expected %q, got %q", i, h, rows1[0][i])
		}
	}

	// Row 1 checks
	if rows1[1][0] != "Иванов Иван Иванович" {
		t.Errorf("row 1 FIO expected 'Иванов Иван Иванович', got %q", rows1[1][0])
	}
	if rows1[1][1] != "112-233-445 95" {
		t.Errorf("row 1 SNILS expected '112-233-445 95', got %q", rows1[1][1])
	}
	if rows1[1][2] != "0.88" {
		t.Errorf("row 1 ML_Score expected '0.88', got %q", rows1[1][2])
	}
	if rows1[1][3] != "A" {
		t.Errorf("row 1 Segment expected 'A', got %q", rows1[1][3])
	}
	if rows1[1][4] != "APPROVED" {
		t.Errorf("row 1 Model_Status expected 'APPROVED', got %q", rows1[1][4])
	}

	// Row 2 checks (unmapped values intact)
	if rows1[2][0] != "Неизвестный Кандидат" {
		t.Errorf("row 2 unmapped FIO expected 'Неизвестный Кандидат', got %q", rows1[2][0])
	}
	if rows1[2][1] != "111-222-333 44" {
		t.Errorf("row 2 unmapped SNILS expected '111-222-333 44', got %q", rows1[2][1])
	}

	// Verify Sheet 2
	rows2, err := rf.GetRows(sheet2)
	if err != nil {
		t.Fatalf("failed to get rows for %s: %v", sheet2, err)
	}
	if len(rows2) < 2 {
		t.Fatalf("expected at least 2 rows in %s, got %d", sheet2, len(rows2))
	}
	if rows2[1][0] != "+7 (916) 123-45-67" {
		t.Errorf("sheet2 row 1 Phone expected '+7 (916) 123-45-67', got %q", rows2[1][0])
	}
	if rows2[1][1] != "TRUE" && rows2[1][1] != "true" && rows2[1][1] != "1" {
		t.Errorf("sheet2 row 1 Email_Verified preserved boolean, got %q", rows2[1][1])
	}
	if rows2[1][2] != "10" {
		t.Errorf("sheet2 row 1 Priority expected '10', got %q", rows2[1][2])
	}
}

func TestRestore_ContextCancelled(t *testing.T) {
	dm := depmap.New("test.csv")
	dm.Put("A", "B")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// CSV context cancellation
	var out bytes.Buffer
	_, err := formats.RestoreCSV(ctx, strings.NewReader("A,C\nA,C\n"), &out, dm)
	if err == nil {
		t.Fatal("expected error on cancelled context in RestoreCSV, got nil")
	}

	// Excel context cancellation
	tmpDir := t.TempDir()
	xlsxPath := filepath.Join(tmpDir, "cancel.xlsx")
	outXlsx := filepath.Join(tmpDir, "out_cancel.xlsx")

	f := excelize.NewFile()
	_ = f.SetCellValue("Sheet1", "A1", "A")
	_ = f.SaveAs(xlsxPath)
	f.Close()

	_, err = formats.RestoreExcel(ctx, xlsxPath, outXlsx, dm, nil)
	if err == nil {
		t.Fatal("expected error on cancelled context in RestoreExcel, got nil")
	}
}

func TestRestore_EdgeCases(t *testing.T) {
	ctx := context.Background()

	// Nil depmap
	var buf bytes.Buffer
	_, err := formats.RestoreCSV(ctx, strings.NewReader("A,B"), &buf, nil)
	if err == nil {
		t.Error("expected error for nil depmap in RestoreCSV")
	}

	_, err = formats.RestoreExcel(ctx, "a.xlsx", "b.xlsx", nil, nil)
	if err == nil {
		t.Error("expected error for nil depmap in RestoreExcel")
	}

	dm := depmap.New("empty.csv")

	// Empty input CSV
	buf.Reset()
	count, err := formats.RestoreCSV(ctx, strings.NewReader(""), &buf, dm)
	if err != nil {
		t.Errorf("unexpected error for empty CSV: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 restored cells for empty CSV, got %d", count)
	}

	// Non-existent Excel file
	_, err = formats.RestoreExcel(ctx, "/path/does/not/exist/at/all.xlsx", "out.xlsx", dm, nil)
	if err == nil {
		t.Error("expected error for non-existent excel file")
	}
}

func TestRestoreExcel_HeaderAwareColumnShift(t *testing.T) {
	tmpDir := t.TempDir()
	origPath := filepath.Join(tmpDir, "original.xlsx")
	anonPath := filepath.Join(tmpDir, "anonymized.xlsx")
	aiModifiedPath := filepath.Join(tmpDir, "ai_modified.xlsx")
	restoredPath := filepath.Join(tmpDir, "restored.xlsx")

	// 1. Create original workbook with headers: ID, FIO, Phone
	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "ID")
	_ = f.SetCellValue(sheet, "B1", "ФИО")
	_ = f.SetCellValue(sheet, "C1", "Телефон")
	_ = f.SetCellValue(sheet, "A2", "1")
	_ = f.SetCellValue(sheet, "B2", "Иванов Иван Иванович")
	_ = f.SetCellValue(sheet, "C2", "+7 999 123-45-67")
	if err := f.SaveAs(origPath); err != nil {
		t.Fatalf("failed to save original xlsx: %v", err)
	}
	f.Close()

	// 2. Anonymize with default tokens and CreateKey=true -> yields anonymized.xlsx and key.depmap
	anon := anonymizer.New("salt-test")
	handler := formats.NewExcelHandler(anon)
	dm := depmap.New("original.xlsx")

	var anonBuf bytes.Buffer
	origBytes, err := os.ReadFile(origPath)
	if err != nil {
		t.Fatalf("failed to read original xlsx: %v", err)
	}
	err = handler.Process(context.Background(), bytes.NewReader(origBytes), &anonBuf, map[string]interface{}{
		"strategy": domain.StrategySemanticTokens,
		"depmap":   dm,
	})
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	if err := os.WriteFile(anonPath, anonBuf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write anonymized xlsx: %v", err)
	}

	anonXl, err := excelize.OpenFile(anonPath)
	if err != nil {
		t.Fatalf("failed to open anonymized xlsx: %v", err)
	}
	fioToken, _ := anonXl.GetCellValue(sheet, "B2")
	phoneToken, _ := anonXl.GetCellValue(sheet, "C2")
	anonXl.Close()

	// 3. Simulate AI model: Insert 2 new columns between ID and FIO, and rename columns
	aiFile := excelize.NewFile()
	aiFile.SetSheetName("Sheet1", sheet)
	_ = aiFile.SetCellValue(sheet, "A1", "ID")
	_ = aiFile.SetCellValue(sheet, "B1", "AI_Score")
	_ = aiFile.SetCellValue(sheet, "C1", "AI_Category")
	_ = aiFile.SetCellValue(sheet, "D1", "Client_FullName_Renamed")
	_ = aiFile.SetCellValue(sheet, "E1", "Mobile_Phone_Renamed")

	_ = aiFile.SetCellValue(sheet, "A2", "1")
	_ = aiFile.SetCellValue(sheet, "B2", "0.95")
	_ = aiFile.SetCellValue(sheet, "C2", "VIP")
	_ = aiFile.SetCellValue(sheet, "D2", fioToken)
	_ = aiFile.SetCellValue(sheet, "E2", phoneToken)

	if err := aiFile.SaveAs(aiModifiedPath); err != nil {
		t.Fatalf("failed to save ai modified xlsx: %v", err)
	}
	aiFile.Close()

	// 4. Run RestoreExcel on the modified file using key.depmap
	ctx := context.Background()
	restoredCount, err := formats.RestoreExcel(ctx, aiModifiedPath, restoredPath, dm, nil)
	if err != nil {
		t.Fatalf("RestoreExcel failed: %v", err)
	}

	// 5. Verify:
	//    - Restore count matches expected (2)
	//    - FIO and Phone are restored to original values despite column shift and renaming
	//    - AI_Score and AI_Category remain intact with AI values
	if restoredCount != 2 {
		t.Errorf("expected 2 restored cells, got %d", restoredCount)
	}

	rf, err := excelize.OpenFile(restoredPath)
	if err != nil {
		t.Fatalf("failed to open restored xlsx: %v", err)
	}
	defer rf.Close()

	idVal, _ := rf.GetCellValue(sheet, "A2")
	if idVal != "1" {
		t.Errorf("expected ID '1', got %q", idVal)
	}

	aiScoreVal, _ := rf.GetCellValue(sheet, "B2")
	if aiScoreVal != "0.95" {
		t.Errorf("expected AI_Score untouched '0.95', got %q", aiScoreVal)
	}

	aiCatVal, _ := rf.GetCellValue(sheet, "C2")
	if aiCatVal != "VIP" {
		t.Errorf("expected AI_Category untouched 'VIP', got %q", aiCatVal)
	}

	fioVal, _ := rf.GetCellValue(sheet, "D2")
	if fioVal != "Иванов Иван Иванович" {
		t.Errorf("expected FIO restored 'Иванов Иван Иванович', got %q", fioVal)
	}

	phoneVal, _ := rf.GetCellValue(sheet, "E2")
	if phoneVal != "+7 999 123-45-67" {
		t.Errorf("expected Phone restored '+7 999 123-45-67', got %q", phoneVal)
	}
}

func TestRestoreCSV_HeaderAwareColumnShift(t *testing.T) {
	origCSV := "ID,ФИО,Телефон\n1,Иванов Иван Иванович,+7 999 123-45-67\n"

	anon := anonymizer.New("salt-test-csv")
	handler := formats.NewCSVHandler(anon)
	dm := depmap.New("clients.csv")

	var anonBuf bytes.Buffer
	err := handler.Process(context.Background(), strings.NewReader(origCSV), &anonBuf, map[string]interface{}{
		"depmap": dm,
	})
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	anonReader := csv.NewReader(&anonBuf)
	anonRecords, err := anonReader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse anon csv: %v", err)
	}
	fioToken := anonRecords[1][1]
	phoneToken := anonRecords[1][2]

	// Simulate AI model inserting columns between ID and renamed ФИО
	aiCSV := "ID,AI_Score,AI_Category,Client_Renamed,Contact\n" +
		"1,0.95,VIP," + fioToken + "," + phoneToken + "\n"

	var restoredBuf bytes.Buffer
	ctx := context.Background()
	restoredCount, err := formats.RestoreCSV(ctx, strings.NewReader(aiCSV), &restoredBuf, dm)
	if err != nil {
		t.Fatalf("RestoreCSV failed: %v", err)
	}

	if restoredCount != 2 {
		t.Errorf("expected 2 restored cells, got %d", restoredCount)
	}

	reader := csv.NewReader(&restoredBuf)
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read restored csv: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[1][0] != "1" {
		t.Errorf("expected ID '1', got %q", rows[1][0])
	}
	if rows[1][1] != "0.95" {
		t.Errorf("expected AI_Score untouched '0.95', got %q", rows[1][1])
	}
	if rows[1][2] != "VIP" {
		t.Errorf("expected AI_Category untouched 'VIP', got %q", rows[1][2])
	}
	if rows[1][3] != "Иванов Иван Иванович" {
		t.Errorf("expected FIO restored 'Иванов Иван Иванович', got %q", rows[1][3])
	}
	if rows[1][4] != "+7 999 123-45-67" {
		t.Errorf("expected Phone restored '+7 999 123-45-67', got %q", rows[1][4])
	}
}

func TestRestoreExcel_TokensInFreeTextAndAIColumns(t *testing.T) {
	tmpDir := t.TempDir()
	origPath := filepath.Join(tmpDir, "orig_tokens.xlsx")
	aiModifiedPath := filepath.Join(tmpDir, "ai_tokens.xlsx")
	restoredPath := filepath.Join(tmpDir, "restored_tokens.xlsx")

	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "ID")
	_ = f.SetCellValue(sheet, "B1", "ФИО")
	_ = f.SetCellValue(sheet, "A2", "1")
	_ = f.SetCellValue(sheet, "B2", "Иванов Иван Иванович")
	if err := f.SaveAs(origPath); err != nil {
		t.Fatalf("failed to save original xlsx: %v", err)
	}
	f.Close()

	anon := anonymizer.New("salt-tokens")
	handler := formats.NewExcelHandler(anon)
	dm := depmap.New("orig_tokens.xlsx")

	var anonBuf bytes.Buffer
	origBytes, err := os.ReadFile(origPath)
	if err != nil {
		t.Fatalf("failed to read original xlsx: %v", err)
	}
	err = handler.Process(context.Background(), bytes.NewReader(origBytes), &anonBuf, map[string]interface{}{
		"strategy": domain.StrategySemanticTokens,
		"depmap":   dm,
	})
	if err != nil {
		t.Fatalf("handler.Process failed: %v", err)
	}

	anonExcel, err := excelize.OpenReader(&anonBuf)
	if err != nil {
		t.Fatalf("failed to open anon excel: %v", err)
	}
	tokenFIO, _ := anonExcel.GetCellValue(sheet, "B2")
	anonExcel.Close()

	// Simulate AI adding column "AI_Comment" with text embedding the token
	aiFile := excelize.NewFile()
	aiFile.SetSheetName("Sheet1", sheet)
	_ = aiFile.SetCellValue(sheet, "A1", "ID")
	_ = aiFile.SetCellValue(sheet, "B1", "AI_Comment")
	_ = aiFile.SetCellValue(sheet, "C1", "Client_Token")

	_ = aiFile.SetCellValue(sheet, "A2", "1")
	_ = aiFile.SetCellValue(sheet, "B2", "Сотрудник "+tokenFIO+" успешно прошел скоринг")
	_ = aiFile.SetCellValue(sheet, "C2", tokenFIO)

	if err := aiFile.SaveAs(aiModifiedPath); err != nil {
		t.Fatalf("failed to save ai modified xlsx: %v", err)
	}
	aiFile.Close()

	ctx := context.Background()
	restoredCount, err := formats.RestoreExcel(ctx, aiModifiedPath, restoredPath, dm, nil)
	if err != nil {
		t.Fatalf("RestoreExcel failed: %v", err)
	}

	if restoredCount != 2 {
		t.Errorf("expected exactly 2 restored cells, got %d", restoredCount)
	}

	rf, err := excelize.OpenFile(restoredPath)
	if err != nil {
		t.Fatalf("failed to open restored xlsx: %v", err)
	}
	defer rf.Close()

	aiCommentVal, _ := rf.GetCellValue(sheet, "B2")
	expectedComment := "Сотрудник Иванов Иван Иванович успешно прошел скоринг"
	if aiCommentVal != expectedComment {
		t.Errorf("expected AI_Comment restored %q, got %q", expectedComment, aiCommentVal)
	}

	fioVal, _ := rf.GetCellValue(sheet, "C2")
	if fioVal != "Иванов Иван Иванович" {
		t.Errorf("expected FIO restored 'Иванов Иван Иванович', got %q", fioVal)
	}
}

func TestRestoreExcel_ShuffledRowsAndTokensInSubstrings(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input_shuffled.xlsx")
	outputPath := filepath.Join(tempDir, "output_restored.xlsx")

	f := excelize.NewFile()
	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "Комментарий_LLM")
	_ = f.SetCellValue(sheet, "B1", "ФИО")
	_ = f.SetCellValue(sheet, "C1", "Паспорт")

	_ = f.SetCellValue(sheet, "A2", "Рекомендация: повысить сотрудника [ФИО_0002] на 1 грейд")
	_ = f.SetCellValue(sheet, "B2", "[ФИО_0002]")
	_ = f.SetCellValue(sheet, "C2", "[Паспорт_0002]")

	_ = f.SetCellValue(sheet, "A3", "Рекомендация: премировать [ФИО_0001]")
	_ = f.SetCellValue(sheet, "B3", "[ФИО_0001]")
	_ = f.SetCellValue(sheet, "C3", "[Паспорт_0001]")

	if err := f.SaveAs(inputPath); err != nil {
		t.Fatalf("failed to save input xlsx: %v", err)
	}
	f.Close()

	dm := depmap.New("test.xlsx")
	dm.Mode = "tokens"
	dm.Put("[ФИО_0001]", "Иванов Иван Иванович")
	dm.Put("[Паспорт_0001]", "4508 123456")
	dm.Put("[ФИО_0002]", "Петров Петр Петрович")
	dm.Put("[Паспорт_0002]", "4510 654321")

	count, err := formats.RestoreExcel(context.Background(), inputPath, outputPath, dm, nil)
	if err != nil {
		t.Fatalf("RestoreExcel failed: %v", err)
	}
	if count < 6 {
		t.Errorf("expected at least 6 replacements, got %d", count)
	}

	rf, err := excelize.OpenFile(outputPath)
	if err != nil {
		t.Fatalf("failed to open restored xlsx: %v", err)
	}
	defer rf.Close()

	// Row 2 checks (Petrov)
	comment2, _ := rf.GetCellValue(sheet, "A2")
	if comment2 != "Рекомендация: повысить сотрудника Петров Петр Петрович на 1 грейд" {
		t.Errorf("expected substring restored in A2, got %q", comment2)
	}
	fio2, _ := rf.GetCellValue(sheet, "B2")
	if fio2 != "Петров Петр Петрович" {
		t.Errorf("expected B2 restored to Петров, got %q", fio2)
	}
	pass2, _ := rf.GetCellValue(sheet, "C2")
	if pass2 != "4510 654321" {
		t.Errorf("expected C2 restored to 4510 654321, got %q", pass2)
	}

	// Row 3 checks (Ivanov)
	comment3, _ := rf.GetCellValue(sheet, "A3")
	if comment3 != "Рекомендация: премировать Иванов Иван Иванович" {
		t.Errorf("expected substring restored in A3, got %q", comment3)
	}
	fio3, _ := rf.GetCellValue(sheet, "B3")
	if fio3 != "Иванов Иван Иванович" {
		t.Errorf("expected B3 restored to Иванов, got %q", fio3)
	}
	pass3, _ := rf.GetCellValue(sheet, "C3")
	if pass3 != "4508 123456" {
		t.Errorf("expected C3 restored to 4508 123456, got %q", pass3)
	}
}

func TestRestoreExcel_ProgressTracking_StreamWriterDimensionA1(t *testing.T) {
	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "stream_writer_dim_a1.xlsx")
	outPath := filepath.Join(tmpDir, "stream_writer_dim_a1_restored.xlsx")

	dm := depmap.New("original.xlsx")
	dm.Put("Синтетика", "Оригинал")

	// Create file using StreamWriter (which leaves Sheet1 dimension as default "A1")
	f := excelize.NewFile()
	sw, err := f.NewStreamWriter("Sheet1")
	if err != nil {
		t.Fatalf("failed to create stream writer: %v", err)
	}
	// Write header + 120 rows
	_ = sw.SetRow("A1", []interface{}{"Колонка1", "Колонка2"})
	for r := 2; r <= 121; r++ {
		coord, _ := excelize.CoordinatesToCellName(1, r)
		_ = sw.SetRow(coord, []interface{}{"Синтетика", r})
	}
	if err := sw.Flush(); err != nil {
		t.Fatalf("failed to flush stream writer: %v", err)
	}
	if err := f.SaveAs(inPath); err != nil {
		t.Fatalf("failed to save stream writer file: %v", err)
	}
	_ = f.Close()

	// Verify that Excelize reports dimension without multi-row range (e.g. "A1")
	checkFile, _ := excelize.OpenFile(inPath)
	dim, _ := checkFile.GetSheetDimension("Sheet1")
	_ = checkFile.Close()
	if strings.Contains(dim, ":") {
		t.Logf("note: dimension unexpectedly contains colon: %s", dim)
	}

	var progressEvents []struct{ Pct, Proc, Total int }
	progressFn := func(pct, proc, total int) {
		progressEvents = append(progressEvents, struct{ Pct, Proc, Total int }{pct, proc, total})
	}

	restored, err := formats.RestoreExcel(context.Background(), inPath, outPath, dm, progressFn)
	if err != nil {
		t.Fatalf("RestoreExcel failed: %v", err)
	}
	if restored != 120 {
		t.Errorf("expected 120 restored cells, got %d", restored)
	}

	if len(progressEvents) == 0 {
		t.Fatalf("expected progress events to be emitted, got none")
	}

	first := progressEvents[0]
	if first.Total != 121 {
		t.Errorf("expected total rows to be detected as 121, got %d", first.Total)
	}

	last := progressEvents[len(progressEvents)-1]
	if last.Pct != 100 {
		t.Errorf("expected final percentage 100, got %d", last.Pct)
	}
	if last.Total != 121 {
		t.Errorf("expected final total to be 121, got %d", last.Total)
	}
	if last.Proc != 121 {
		t.Errorf("expected final proc to be 121, got %d", last.Proc)
	}
}

func TestRestoreExcel_MultiTierHeaderAndMergesPreserved(t *testing.T) {
	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "input_merges.xlsx")
	outPath := filepath.Join(tmpDir, "output_merges.xlsx")

	f := excelize.NewFile()
	sheet := "DataSheet"
	f.SetSheetName("Sheet1", sheet)

	// Merged category header in row 1
	_ = f.SetCellValue(sheet, "B1", "Категория 1")
	_ = f.MergeCell(sheet, "B1", "C1")

	// Leaf headers in row 2
	_ = f.SetCellValue(sheet, "A2", "Участник")
	_ = f.SetCellValue(sheet, "B2", "Балл 1")
	_ = f.SetCellValue(sheet, "C2", "Балл 2")

	// Data in row 3
	_ = f.SetCellValue(sheet, "A3", "[ФИО_0001]")
	_ = f.SetCellValue(sheet, "B3", "5")
	_ = f.SetCellValue(sheet, "C3", "9")

	if err := f.SaveAs(inPath); err != nil {
		t.Fatal(err)
	}
	f.Close()

	dm := depmap.New(inPath)
	dm.Put("[ФИО_0001]", "Сидоров Сидор Сидорович")

	restored, err := formats.RestoreExcel(context.Background(), inPath, outPath, dm, nil)
	if err != nil {
		t.Fatalf("RestoreExcel failed: %v", err)
	}
	if restored != 1 {
		t.Errorf("expected 1 restored cell, got %d", restored)
	}

	resFile, err := excelize.OpenFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer resFile.Close()

	// Verify merged cells were preserved
	merges, err := resFile.GetMergeCells(sheet)
	if err != nil {
		t.Fatal(err)
	}
	if len(merges) != 1 {
		t.Fatalf("expected 1 merge range, got %d", len(merges))
	}
	if merges[0].GetStartAxis() != "B1" || merges[0].GetEndAxis() != "C1" {
		t.Errorf("expected B1:C1 merge, got %s:%s", merges[0].GetStartAxis(), merges[0].GetEndAxis())
	}

	valA3, _ := resFile.GetCellValue(sheet, "A3")
	if valA3 != "Сидоров Сидор Сидорович" {
		t.Errorf("expected restored FIO, got %q", valA3)
	}
}

func TestRestoreExcel_MultiTierHeader_TokenRestoration(t *testing.T) {
	tmpDir := t.TempDir()
	inPath := filepath.Join(tmpDir, "input_multitier.xlsx")
	outPath := filepath.Join(tmpDir, "output_multitier.xlsx")

	f := excelize.NewFile()
	sheet := "Sheet1"

	// Merged category header in row 1
	_ = f.SetCellValue(sheet, "A1", "Личные данные")
	_ = f.SetCellValue(sheet, "B1", "Личные данные")
	_ = f.MergeCell(sheet, "A1", "B1")

	// Leaf headers in row 2
	_ = f.SetCellValue(sheet, "A2", "Участник")
	_ = f.SetCellValue(sheet, "B2", "Город")

	// Data in row 3
	_ = f.SetCellValue(sheet, "A3", "[ФИО_0001]")
	_ = f.SetCellValue(sheet, "B3", "Москва")

	if err := f.SaveAs(inPath); err != nil {
		t.Fatal(err)
	}
	f.Close()

	dm := depmap.New(inPath)
	dm.Put("[ФИО_0001]", "Иванов Иван Иванович")

	restored, err := formats.RestoreExcel(context.Background(), inPath, outPath, dm, nil)
	if err != nil {
		t.Fatalf("RestoreExcel failed: %v", err)
	}
	if restored != 1 {
		t.Errorf("expected 1 restored cell, got %d", restored)
	}

	resFile, err := excelize.OpenFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer resFile.Close()

	valA3, _ := resFile.GetCellValue(sheet, "A3")
	if valA3 != "Иванов Иван Иванович" {
		t.Errorf("expected restored FIO from token, got %q", valA3)
	}

	valB3, _ := resFile.GetCellValue(sheet, "B3")
	if valB3 != "Москва" {
		t.Errorf("expected untouched city 'Москва', got %q", valB3)
	}

	// Verify merged cells preserved
	merges, err := resFile.GetMergeCells(sheet)
	if err != nil || len(merges) == 0 {
		t.Errorf("expected merged cells to be preserved, got %v", merges)
	}
}

func TestRestoreExcel_ExhaustiveExternalModifications(t *testing.T) {
	tmpDir := t.TempDir()
	origPath := filepath.Join(tmpDir, "employees_orig.xlsx")
	anonPath := filepath.Join(tmpDir, "employees_anon.xlsx")
	modifiedPath := filepath.Join(tmpDir, "employees_ai_processed.xlsx")
	restoredPath := filepath.Join(tmpDir, "employees_restored.xlsx")

	// 1. Create original Excel file with multiple PII columns
	origFile := excelize.NewFile()
	sheet := "Сотрудники"
	origFile.SetSheetName("Sheet1", sheet)

	headers := []string{"Табельный", "ФИО", "СНИЛС", "Телефон", "Паспорт", "Отдел", "Оклад"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = origFile.SetCellValue(sheet, cell, h)
	}

	origRows := [][]interface{}{
		{"00101", "Иванов Иван Иванович", "112-233-445 95", "+7 (999) 111-22-33", "4508 123456", "Разработка", 150000},
		{"00102", "Петрова Анна Сергеевна", "223-344-556 01", "+7 (999) 222-33-44", "4510 654321", "Аналитика", 140000},
		{"00103", "Сидоров Алексей Петрович", "334-455-667 12", "+7 (999) 333-44-55", "4512 987654", "Безопасность", 160000},
	}

	for rIdx, rData := range origRows {
		for cIdx, val := range rData {
			cell, _ := excelize.CoordinatesToCellName(cIdx+1, rIdx+2)
			_ = origFile.SetCellValue(sheet, cell, val)
		}
	}

	if err := origFile.SaveAs(origPath); err != nil {
		t.Fatalf("failed to save original xlsx: %v", err)
	}
	origFile.Close()

	// 2. Anonymize with StrategySemanticTokens and save key
	anon := anonymizer.New("salt-comprehensive-test")
	handler := formats.NewExcelHandler(anon)
	dm := depmap.New(origPath)

	origBytes, err := os.ReadFile(origPath)
	if err != nil {
		t.Fatalf("failed to read original xlsx: %v", err)
	}

	var anonBuf bytes.Buffer
	err = handler.Process(context.Background(), bytes.NewReader(origBytes), &anonBuf, map[string]interface{}{
		"strategy": domain.StrategySemanticTokens,
		"depmap":   dm,
	})
	if err != nil {
		t.Fatalf("ExcelHandler.Process failed: %v", err)
	}

	if err := os.WriteFile(anonPath, anonBuf.Bytes(), 0644); err != nil {
		t.Fatalf("failed to write anonymized xlsx: %v", err)
	}

	// Read anonymized tokens from anonPath
	anonXl, err := excelize.OpenFile(anonPath)
	if err != nil {
		t.Fatalf("failed to open anonymized file: %v", err)
	}
	defer anonXl.Close()

	anonFIO1, _ := anonXl.GetCellValue(sheet, "B2")
	anonSNILS1, _ := anonXl.GetCellValue(sheet, "C2")
	anonPhone1, _ := anonXl.GetCellValue(sheet, "D2")
	anonPass1, _ := anonXl.GetCellValue(sheet, "E2")

	anonFIO2, _ := anonXl.GetCellValue(sheet, "B3")
	anonSNILS2, _ := anonXl.GetCellValue(sheet, "C3")
	anonPhone2, _ := anonXl.GetCellValue(sheet, "D3")
	anonPass2, _ := anonXl.GetCellValue(sheet, "E3")

	anonFIO3, _ := anonXl.GetCellValue(sheet, "B4")
	anonSNILS3, _ := anonXl.GetCellValue(sheet, "C4")
	anonPhone3, _ := anonXl.GetCellValue(sheet, "D4")
	anonPass3, _ := anonXl.GetCellValue(sheet, "E4")

	// 3. Simulate deep AI / user modifications:
	// - Renamed headers:
	//     "Табельный" -> "Emp_ID"
	//     "ФИО" -> "Full_Name_Renamed"
	//     "Телефон" -> "Mobile_Contact"
	//     "Паспорт" -> "Doc_Series_Number"
	// - Shifted / Reordered columns:
	//     Put "Mobile_Contact" BEFORE "Full_Name_Renamed"
	// - Inserted NEW columns:
	//     Col A: "Batch_ID" (inserted before Emp_ID)
	//     Col E: "ML_Performance_Score" (inserted between columns)
	//     Col F: "AI_Recommendation" (contains LLM sentence with token!)
	//     Col J: "Risk_Tier"
	//     Col K: "Audit_Flag"
	// - Shuffled / Reordered rows:
	//     Row 2 in modified = Employee 3 (Sidorov)
	//     Row 3 in modified = Employee 1 (Ivanov)
	//     Row 4 in modified = Employee 2 (Petrova)
	// - Appended a BRAND NEW row (Row 5) with a new hire not in original mapping
	// - Appended a summary row (Row 6)
	aiFile := excelize.NewFile()
	aiFile.SetSheetName("Sheet1", sheet)

	modifiedHeaders := []string{
		"Batch_ID",               // 1: new col
		"Emp_ID",                 // 2: renamed
		"Mobile_Contact",         // 3: moved + renamed
		"Full_Name_Renamed",      // 4: moved + renamed
		"ML_Performance_Score",   // 5: new col
		"AI_Recommendation",      // 6: new col with embedded token in text!
		"СНИЛС",                  // 7: original header preserved
		"Doc_Series_Number",      // 8: renamed
		"Отдел",                  // 9: original header
		"Оклад",                  // 10: original header
		"Risk_Tier",              // 11: new col
		"Audit_Flag",             // 12: new col
	}

	for i, h := range modifiedHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = aiFile.SetCellValue(sheet, cell, h)
	}

	// Modified Row 2: Employee 3 (Sidorov)
	aiRow2 := []interface{}{
		"BATCH-2026-A",
		"00103",
		anonPhone3,
		anonFIO3,
		0.98,
		"Кандидат " + anonFIO3 + " рекомендован к повышению на ведущего эксперта",
		anonSNILS3,
		anonPass3,
		"Безопасность",
		160000,
		"Low",
		true,
	}
	for i, v := range aiRow2 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = aiFile.SetCellValue(sheet, cell, v)
	}

	// Modified Row 3: Employee 1 (Ivanov)
	aiRow3 := []interface{}{
		"BATCH-2026-A",
		"00101",
		anonPhone1,
		anonFIO1,
		0.89,
		"Сотрудник " + anonFIO1 + " успешно прошел ежегодную аттестацию",
		anonSNILS1,
		anonPass1,
		"Разработка",
		150000,
		"Medium",
		false,
	}
	for i, v := range aiRow3 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 3)
		_ = aiFile.SetCellValue(sheet, cell, v)
	}

	// Modified Row 4: Employee 2 (Petrova)
	aiRow4 := []interface{}{
		"BATCH-2026-A",
		"00102",
		anonPhone2,
		anonFIO2,
		0.94,
		"Назначить премию для " + anonFIO2 + " по итогам квартала",
		anonSNILS2,
		anonPass2,
		"Аналитика",
		140000,
		"Low",
		true,
	}
	for i, v := range aiRow4 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 4)
		_ = aiFile.SetCellValue(sheet, cell, v)
	}

	// Modified Row 5: Brand new candidate (not in depmap)
	aiRow5 := []interface{}{
		"BATCH-2026-B",
		"00999",
		"+7 (999) 000-00-00",
		"Кузнецов Станислав",
		0.75,
		"Новый кандидат на стажировку",
		"000-000-000 00",
		"4599 000000",
		"Стажеры",
		50000,
		"High",
		false,
	}
	for i, v := range aiRow5 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 5)
		_ = aiFile.SetCellValue(sheet, cell, v)
	}

	// Modified Row 6: Summary Row
	aiRow6 := []interface{}{
		"ИТОГО", "", "", "", 0.89, "Всего обработано записей: 4", "", "", "", 500000, "", "",
	}
	for i, v := range aiRow6 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 6)
		_ = aiFile.SetCellValue(sheet, cell, v)
	}

	if err := aiFile.SaveAs(modifiedPath); err != nil {
		t.Fatalf("failed to save modified xlsx: %v", err)
	}
	aiFile.Close()

	// 4. Run RestoreExcel on the heavily modified file
	ctx := context.Background()
	restoredCount, err := formats.RestoreExcel(ctx, modifiedPath, restoredPath, dm, nil)
	if err != nil {
		t.Fatalf("RestoreExcel failed: %v", err)
	}

	// Expect 15 restorations:
	// Row 2: Phone, FIO, AI_Recommendation (substring), SNILS, Passport = 5
	// Row 3: Phone, FIO, AI_Recommendation (substring), SNILS, Passport = 5
	// Row 4: Phone, FIO, AI_Recommendation (substring), SNILS, Passport = 5
	if restoredCount != 15 {
		t.Errorf("expected exactly 15 restored occurrences, got %d", restoredCount)
	}

	// 5. Inspect and rigorously verify restored file
	rf, err := excelize.OpenFile(restoredPath)
	if err != nil {
		t.Fatalf("failed to open restored file: %v", err)
	}
	defer rf.Close()

	// Check headers untouched
	for i, expectedHeader := range modifiedHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		actualVal, _ := rf.GetCellValue(sheet, cell)
		if actualVal != expectedHeader {
			t.Errorf("header col %d mismatch: expected %q, got %q", i+1, expectedHeader, actualVal)
		}
	}

	// Row 2 Checks (Sidorov)
	valBatch2, _ := rf.GetCellValue(sheet, "A2")
	if valBatch2 != "BATCH-2026-A" {
		t.Errorf("row 2 Batch_ID corrupted: got %q", valBatch2)
	}
	valEmp2, _ := rf.GetCellValue(sheet, "B2")
	if valEmp2 != "00103" {
		t.Errorf("row 2 Emp_ID corrupted: got %q", valEmp2)
	}
	valPhone2, _ := rf.GetCellValue(sheet, "C2")
	if valPhone2 != "+7 (999) 333-44-55" {
		t.Errorf("row 2 Phone restored mismatch: expected '+7 (999) 333-44-55', got %q", valPhone2)
	}
	valFIO2, _ := rf.GetCellValue(sheet, "D2")
	if valFIO2 != "Сидоров Алексей Петрович" {
		t.Errorf("row 2 FIO restored mismatch: expected 'Сидоров Алексей Петрович', got %q", valFIO2)
	}
	valScore2, _ := rf.GetCellValue(sheet, "E2")
	if valScore2 != "0.98" {
		t.Errorf("row 2 ML_Performance_Score corrupted: got %q", valScore2)
	}
	valRec2, _ := rf.GetCellValue(sheet, "F2")
	expectedRec2 := "Кандидат Сидоров Алексей Петрович рекомендован к повышению на ведущего эксперта"
	if valRec2 != expectedRec2 {
		t.Errorf("row 2 AI_Recommendation substring mismatch: expected %q, got %q", expectedRec2, valRec2)
	}
	valSNILS2, _ := rf.GetCellValue(sheet, "G2")
	if valSNILS2 != "334-455-667 12" {
		t.Errorf("row 2 SNILS restored mismatch: expected '334-455-667 12', got %q", valSNILS2)
	}
	valPass2, _ := rf.GetCellValue(sheet, "H2")
	if valPass2 != "4512 987654" {
		t.Errorf("row 2 Passport restored mismatch: expected '4512 987654', got %q", valPass2)
	}
	valTier2, _ := rf.GetCellValue(sheet, "K2")
	if valTier2 != "Low" {
		t.Errorf("row 2 Risk_Tier corrupted: got %q", valTier2)
	}

	// Row 3 Checks (Ivanov)
	valPhone3, _ := rf.GetCellValue(sheet, "C3")
	if valPhone3 != "+7 (999) 111-22-33" {
		t.Errorf("row 3 Phone restored mismatch: expected '+7 (999) 111-22-33', got %q", valPhone3)
	}
	valFIO3, _ := rf.GetCellValue(sheet, "D3")
	if valFIO3 != "Иванов Иван Иванович" {
		t.Errorf("row 3 FIO restored mismatch: expected 'Иванов Иван Иванович', got %q", valFIO3)
	}
	valRec3, _ := rf.GetCellValue(sheet, "F3")
	expectedRec3 := "Сотрудник Иванов Иван Иванович успешно прошел ежегодную аттестацию"
	if valRec3 != expectedRec3 {
		t.Errorf("row 3 AI_Recommendation substring mismatch: expected %q, got %q", expectedRec3, valRec3)
	}
	valSNILS3, _ := rf.GetCellValue(sheet, "G3")
	if valSNILS3 != "112-233-445 95" {
		t.Errorf("row 3 SNILS restored mismatch: expected '112-233-445 95', got %q", valSNILS3)
	}

	// Row 4 Checks (Petrova)
	valPhone4, _ := rf.GetCellValue(sheet, "C4")
	if valPhone4 != "+7 (999) 222-33-44" {
		t.Errorf("row 4 Phone restored mismatch: expected '+7 (999) 222-33-44', got %q", valPhone4)
	}
	valFIO4, _ := rf.GetCellValue(sheet, "D4")
	if valFIO4 != "Петрова Анна Сергеевна" {
		t.Errorf("row 4 FIO restored mismatch: expected 'Петрова Анна Сергеевна', got %q", valFIO4)
	}
	valRec4, _ := rf.GetCellValue(sheet, "F4")
	expectedRec4 := "Назначить премию для Петрова Анна Сергеевна по итогам квартала"
	if valRec4 != expectedRec4 {
		t.Errorf("row 4 AI_Recommendation substring mismatch: expected %q, got %q", expectedRec4, valRec4)
	}

	// Row 5 Checks: Unmapped new candidate preserved as is
	valFIO5, _ := rf.GetCellValue(sheet, "D5")
	if valFIO5 != "Кузнецов Станислав" {
		t.Errorf("row 5 unmapped candidate corrupted: got %q", valFIO5)
	}

	// Row 6 Checks: Summary row preserved
	valSumA, _ := rf.GetCellValue(sheet, "A6")
	if valSumA != "ИТОГО" {
		t.Errorf("row 6 summary label corrupted: got %q", valSumA)
	}
	valSumF, _ := rf.GetCellValue(sheet, "F6")
	if valSumF != "Всего обработано записей: 4" {
		t.Errorf("row 6 summary text corrupted: got %q", valSumF)
	}
}

func TestRestoreCSV_ExhaustiveExternalModifications(t *testing.T) {
	origCSV := "Табельный,ФИО,СНИЛС,Телефон\n" +
		"001,Иванов Иван Иванович,112-233-445 95,+7 999 111-22-33\n" +
		"002,Петрова Анна Сергеевна,223-344-556 01,+7 999 222-33-44\n"

	anon := anonymizer.New("salt-csv-comprehensive")
	handler := formats.NewCSVHandler(anon)
	dm := depmap.New("staff.csv")

	var anonBuf bytes.Buffer
	err := handler.Process(context.Background(), strings.NewReader(origCSV), &anonBuf, map[string]interface{}{
		"strategy": domain.StrategySemanticTokens,
		"depmap":   dm,
	})
	if err != nil {
		t.Fatalf("CSVHandler.Process failed: %v", err)
	}

	reader := csv.NewReader(&anonBuf)
	anonRows, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read anon csv: %v", err)
	}

	fio1 := anonRows[1][1]
	snils1 := anonRows[1][2]
	phone1 := anonRows[1][3]

	fio2 := anonRows[2][1]
	snils2 := anonRows[2][2]
	phone2 := anonRows[2][3]

	// Simulate AI Pipeline modifying CSV:
	// - Column 1: "AI_Run_ID" (new column)
	// - Column 2: "Staff_Code" (renamed)
	// - Column 3: "Contact_Phone" (moved before FIO, renamed)
	// - Column 4: "Employee_Name" (renamed)
	// - Column 5: "AI_Verdict" (new column with embedded token)
	// - Column 6: "Insurance_Number" (renamed SNILS)
	// - Column 7: "Score" (new column)
	// - Rows shuffled: Row 2 (Petrova) comes first, Row 1 (Ivanov) comes second
	// - Row 3 added: brand new external record
	modifiedCSV := "AI_Run_ID,Staff_Code,Contact_Phone,Employee_Name,AI_Verdict,Insurance_Number,Score\n" +
		"RUN-01,002," + phone2 + "," + fio2 + ",\"Кандидат " + fio2 + " рекомендован к найму\"," + snils2 + ",0.98\n" +
		"RUN-01,001," + phone1 + "," + fio1 + ",\"Сотрудник " + fio1 + " прошел проверку\"," + snils1 + ",0.91\n" +
		"RUN-01,999,+7 000 000-00-00,Новиков Игорь,Внешний кандидат не в базе,000-000-000 00,0.50\n"

	var restoredBuf bytes.Buffer
	ctx := context.Background()
	restoredCount, err := formats.RestoreCSV(ctx, strings.NewReader(modifiedCSV), &restoredBuf, dm)
	if err != nil {
		t.Fatalf("RestoreCSV failed: %v", err)
	}

	// 2 rows * 4 items (phone, fio, verdict-token, snils) = 8
	if restoredCount != 8 {
		t.Errorf("expected 8 restored occurrences, got %d", restoredCount)
	}

	resReader := csv.NewReader(&restoredBuf)
	rows, err := resReader.ReadAll()
	if err != nil {
		t.Fatalf("failed to parse restored csv: %v", err)
	}

	// Header verification
	expectedHeaders := []string{"AI_Run_ID", "Staff_Code", "Contact_Phone", "Employee_Name", "AI_Verdict", "Insurance_Number", "Score"}
	for i, h := range expectedHeaders {
		if rows[0][i] != h {
			t.Errorf("header col %d mismatch: want %q, got %q", i, h, rows[0][i])
		}
	}

	// Row 1 checks (Petrova)
	if rows[1][0] != "RUN-01" {
		t.Errorf("row 1 AI_Run_ID corrupted: %q", rows[1][0])
	}
	if rows[1][2] != "+7 999 222-33-44" {
		t.Errorf("row 1 Phone mismatch: want '+7 999 222-33-44', got %q", rows[1][2])
	}
	if rows[1][3] != "Петрова Анна Сергеевна" {
		t.Errorf("row 1 FIO mismatch: want 'Петрова Анна Сергеевна', got %q", rows[1][3])
	}
	expectedVerdict1 := "Кандидат Петрова Анна Сергеевна рекомендован к найму"
	if rows[1][4] != expectedVerdict1 {
		t.Errorf("row 1 AI_Verdict substring mismatch: want %q, got %q", expectedVerdict1, rows[1][4])
	}
	if rows[1][5] != "223-344-556 01" {
		t.Errorf("row 1 SNILS mismatch: want '223-344-556 01', got %q", rows[1][5])
	}
	if rows[1][6] != "0.98" {
		t.Errorf("row 1 Score corrupted: %q", rows[1][6])
	}

	// Row 2 checks (Ivanov)
	if rows[2][2] != "+7 999 111-22-33" {
		t.Errorf("row 2 Phone mismatch: want '+7 999 111-22-33', got %q", rows[2][2])
	}
	if rows[2][3] != "Иванов Иван Иванович" {
		t.Errorf("row 2 FIO mismatch: want 'Иванов Иван Иванович', got %q", rows[2][3])
	}
	expectedVerdict2 := "Сотрудник Иванов Иван Иванович прошел проверку"
	if rows[2][4] != expectedVerdict2 {
		t.Errorf("row 2 AI_Verdict substring mismatch: want %q, got %q", expectedVerdict2, rows[2][4])
	}
	if rows[2][5] != "112-233-445 95" {
		t.Errorf("row 2 SNILS mismatch: want '112-233-445 95', got %q", rows[2][5])
	}

	// Row 3 checks: Unmapped candidate
	if rows[3][3] != "Новиков Игорь" {
		t.Errorf("row 3 unmapped name corrupted: %q", rows[3][3])
	}
}






