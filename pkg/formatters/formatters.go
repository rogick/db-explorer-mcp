package formatters

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
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

	case "llm":
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
