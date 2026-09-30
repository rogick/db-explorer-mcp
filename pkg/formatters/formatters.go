package formatters

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/rogick/db-explorer-mcp/pkg/db"
)

var invalidXmlTagChar = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// StringifyRows converte os valores de um slice de mapas para uma representação em string ou nil,
// imitando o comportamento do TS (String(val)).
func StringifyRows(rows []map[string]interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		newRow := make(map[string]interface{}, len(row))
		for k, v := range row {
			if v == nil {
				newRow[k] = nil
			} else {
				switch val := v.(type) {
				case []byte:
					newRow[k] = string(val)
				case time.Time:
					newRow[k] = val.Format("2006-01-02 15:04:05")
				default:
					newRow[k] = fmt.Sprintf("%v", val)
				}
			}
		}
		result = append(result, newRow)
	}
	return result
}

func FormatOutput(rows []map[string]interface{}, format string, columnOrder []string) (string, error) {
	stringifiedRows := StringifyRows(rows)

	switch strings.ToLower(format) {
	case "xml":
		var sb strings.Builder
		sb.WriteString("<results>\n")
		for _, row := range stringifiedRows {
			sb.WriteString("  <row>\n")
			keys := getKeys(row, columnOrder)
			for _, k := range keys {
				val := row[k]
				safeKey := invalidXmlTagChar.ReplaceAllString(k, "_")
				strVal := ""
				if val != nil {
					strVal = fmt.Sprintf("%v", val)
					strVal = strings.ReplaceAll(strVal, "&", "&amp;")
					strVal = strings.ReplaceAll(strVal, "<", "&lt;")
					strVal = strings.ReplaceAll(strVal, ">", "&gt;")
				}
				sb.WriteString(fmt.Sprintf("    <%s>%s</%s>\n", safeKey, strVal, safeKey))
			}
			sb.WriteString("  </row>\n")
		}
		sb.WriteString("</results>")
		return sb.String(), nil

	case "csv":
		if len(stringifiedRows) == 0 && len(columnOrder) == 0 {
			return "", nil
		}
		var keys []string
		if len(stringifiedRows) > 0 {
			keys = getKeys(stringifiedRows[0], columnOrder)
		} else {
			keys = columnOrder
		}

		var sb strings.Builder
		writer := csv.NewWriter(&sb)

		if err := writer.Write(keys); err != nil {
			return "", fmt.Errorf("erro ao formatar CSV: %w", err)
		}

		for _, row := range stringifiedRows {
			record := make([]string, len(keys))
			for i, k := range keys {
				val := row[k]
				if val != nil {
					record[i] = fmt.Sprintf("%v", val)
				}
			}
			if err := writer.Write(record); err != nil {
				return "", fmt.Errorf("erro ao formatar CSV: %w", err)
			}
		}

		writer.Flush()
		if err := writer.Error(); err != nil {
			return "", fmt.Errorf("erro ao finalizar CSV: %w", err)
		}
		return sb.String(), nil

	case "md", "markdown", "llm":
		if len(stringifiedRows) == 0 {
			return "Nenhum resultado retornado.", nil
		}
		keys := getKeys(stringifiedRows[0], columnOrder)
		var sb strings.Builder
		sb.WriteString("| " + strings.Join(keys, " | ") + " |\n")

		separators := make([]string, len(keys))
		for i := range separators {
			separators[i] = "---"
		}
		sb.WriteString("| " + strings.Join(separators, " | ") + " |\n")

		for _, row := range stringifiedRows {
			rowVals := make([]string, len(keys))
			for i, k := range keys {
				val := row[k]
				strVal := ""
				if val != nil {
					strVal = fmt.Sprintf("%v", val)
				}
				strVal = strings.ReplaceAll(strVal, "|", "\\|")
				strVal = strings.ReplaceAll(strVal, "\n", " ")
				rowVals[i] = strVal
			}
			sb.WriteString("| " + strings.Join(rowVals, " | ") + " |\n")
		}
		return sb.String(), nil

	case "toon":
		if len(stringifiedRows) == 0 {
			return "results[0]{}:\n", nil
		}
		keys := getKeys(stringifiedRows[0], columnOrder)
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("results[%d]{%s}:\n", len(stringifiedRows), strings.Join(keys, ",")))

		for _, row := range stringifiedRows {
			rowVals := make([]string, len(keys))
			for i, k := range keys {
				val := row[k]
				if val == nil {
					rowVals[i] = ""
					continue
				}
				strVal := fmt.Sprintf("%v", val)
				if strings.Contains(strVal, ",") || strings.Contains(strVal, "\n") || strings.Contains(strVal, "\"") {
					strVal = fmt.Sprintf("\"%s\"", strings.ReplaceAll(strVal, "\"", "\"\""))
				}
				rowVals[i] = strVal
			}
			sb.WriteString(fmt.Sprintf("  %s\n", strings.Join(rowVals, ",")))
		}
		return strings.TrimRight(sb.String(), "\n"), nil

	default: // "json"
		data, err := json.MarshalIndent(stringifiedRows, "", "  ")
		if err != nil {
			return "", fmt.Errorf("erro ao formatar JSON: %w", err)
		}
		return string(data), nil
	}
}

func getKeys(row map[string]interface{}, columnOrder []string) []string {
	if len(columnOrder) > 0 {
		return columnOrder
	}
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	return keys
}

func FormatTableSchemaMarkdown(schema *db.TableSchema, detailLevel string) string {
	if schema == nil {
		return "Nenhum schema disponível."
	}

	detailLevel = db.NormalizeDetailLevel(detailLevel)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### Tabela: %s\n\n", schema.Table))
	sb.WriteString("#### Colunas\n")

	var headers []string
	switch detailLevel {
	case "basic":
		headers = []string{"Coluna", "Tipo", "Nullable", "PK"}
	case "standard":
		headers = []string{"Coluna", "Tipo", "Tamanho", "Precisão", "Nullable", "Padrão", "PK"}
	case "detailed":
		headers = []string{"Coluna", "Tipo", "Tamanho", "Precisão", "Nullable", "Padrão", "PK", "Checks"}
	}

	sb.WriteString("| " + strings.Join(headers, " | ") + " |\n")
	seps := make([]string, len(headers))
	for i := range seps {
		seps[i] = "---"
	}
	sb.WriteString("| " + strings.Join(seps, " | ") + " |\n")

	for _, col := range schema.Columns {
		nullableStr := "NÃO"
		if col.Nullable {
			nullableStr = "SIM"
		}
		pkStr := "NÃO"
		if col.PrimaryKey {
			pkStr = "SIM"
		}

		lenStr := "-"
		if col.Length != nil {
			lenStr = fmt.Sprintf("%d", *col.Length)
		}

		precStr := "-"
		if col.Precision != nil {
			if col.Scale != nil && *col.Scale > 0 {
				precStr = fmt.Sprintf("%d,%d", *col.Precision, *col.Scale)
			} else {
				precStr = fmt.Sprintf("%d", *col.Precision)
			}
		}

		defStr := "-"
		if col.DefaultValue != nil && *col.DefaultValue != "" {
			defStr = *col.DefaultValue
		}

		checksStr := "-"
		if len(col.Checks) > 0 {
			checksStr = strings.Join(col.Checks, "; ")
		}

		var rowVals []string
		switch detailLevel {
		case "basic":
			rowVals = []string{col.Column, col.Type, nullableStr, pkStr}
		case "standard":
			rowVals = []string{col.Column, col.Type, lenStr, precStr, nullableStr, defStr, pkStr}
		case "detailed":
			rowVals = []string{col.Column, col.Type, lenStr, precStr, nullableStr, defStr, pkStr, checksStr}
		}

		for i := range rowVals {
			rowVals[i] = strings.ReplaceAll(rowVals[i], "|", "\\|")
			rowVals[i] = strings.ReplaceAll(rowVals[i], "\n", " ")
		}

		sb.WriteString("| " + strings.Join(rowVals, " | ") + " |\n")
	}

	// Constraints
	if schema.PrimaryKey != nil && len(schema.PrimaryKey.Columns) > 0 {
		sb.WriteString("\n#### Primary Key\n")
		name := schema.PrimaryKey.Name
		if name == "" {
			name = "PK"
		}
		sb.WriteString(fmt.Sprintf("- **%s**: %s\n", name, strings.Join(schema.PrimaryKey.Columns, ", ")))
	}

	if len(schema.ForeignKeys) > 0 {
		sb.WriteString("\n#### Foreign Keys\n")
		for _, fk := range schema.ForeignKeys {
			name := fk.Name
			if name == "" {
				name = "FK"
			}
			refCols := strings.Join(fk.ReferencedColumns, ", ")
			if refCols != "" {
				refCols = "(" + refCols + ")"
			}
			sb.WriteString(fmt.Sprintf("- **%s**: (%s) -> %s%s\n",
				name, strings.Join(fk.Columns, ", "), fk.ReferencedTable, refCols))
		}
	}

	if len(schema.UniqueConstraints) > 0 {
		sb.WriteString("\n#### Unique Constraints\n")
		for _, uq := range schema.UniqueConstraints {
			name := uq.Name
			if name == "" {
				name = "UQ"
			}
			sb.WriteString(fmt.Sprintf("- **%s**: (%s)\n", name, strings.Join(uq.Columns, ", ")))
		}
	}

	if detailLevel == "detailed" && len(schema.CheckConstraints) > 0 {
		sb.WriteString("\n#### Check Constraints\n")
		for _, chk := range schema.CheckConstraints {
			name := chk.Name
			if name == "" {
				name = "CHK"
			}
			colsInfo := ""
			if len(chk.Columns) > 0 {
				colsInfo = fmt.Sprintf(" (colunas: %s)", strings.Join(chk.Columns, ", "))
			}
			sb.WriteString(fmt.Sprintf("- **%s**: %s%s\n", name, chk.Clause, colsInfo))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}
