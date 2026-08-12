package security

import (
	"fmt"
	"strings"
	"unicode"
)

type CheckResult struct {
	IsSafe   bool   `json:"isSafe"`
	ErrorMsg string `json:"errorMsg"`
}

// StripCommentsAndLiterals remove comentários (-- e /* */) e substitui literais de string por espaço
// para evitar que palavras-chave como DROP ou DELETE dentro de strings 'DELETE FROM' afetem a checagem,
// ou que comentários ocultem comandos.
func stripCommentsAndLiterals(query string) string {
	runes := []rune(query)
	n := len(runes)
	var sb strings.Builder
	sb.Grow(n)

	i := 0
	for i < n {
		// Comentário de bloco /* ... */
		if i+1 < n && runes[i] == '/' && runes[i+1] == '*' {
			i += 2
			for i+1 < n && !(runes[i] == '*' && runes[i+1] == '/') {
				i++
			}
			if i+1 < n {
				i += 2
			}
			sb.WriteRune(' ')
			continue
		}

		// Comentário de linha -- ou #
		if (i+1 < n && runes[i] == '-' && runes[i+1] == '-') || runes[i] == '#' {
			for i < n && runes[i] != '\n' && runes[i] != '\r' {
				i++
			}
			sb.WriteRune(' ')
			continue
		}

		// String literal '...'
		if runes[i] == '\'' {
			i++
			for i < n {
				if runes[i] == '\'' {
					if i+1 < n && runes[i+1] == '\'' { // aspas escapadas ''
						i += 2
						continue
					}
					i++
					break
				}
				if runes[i] == '\\' {
					i += 2
					continue
				}
				i++
			}
			sb.WriteString(" '' ")
			continue
		}

		// String literal "..." (se não for identificador em ANSI SQL, trata seguro)
		if runes[i] == '"' {
			i++
			for i < n {
				if runes[i] == '"' {
					if i+1 < n && runes[i+1] == '"' {
						i += 2
						continue
					}
					i++
					break
				}
				if runes[i] == '\\' {
					i += 2
					continue
				}
				i++
			}
			sb.WriteString(" \"\" ")
			continue
		}

		sb.WriteRune(runes[i])
		i++
	}

	return sb.String()
}

// extractStatementVerbs retorna o primeiro verbo/instrução de cada comando SQL separado por ponto-e-vírgula.
func extractStatementVerbs(cleanQuery string) []string {
	statements := strings.Split(cleanQuery, ";")
	var verbs []string

	for _, stmt := range statements {
		tokens := strings.Fields(stmt)
		if len(tokens) == 0 {
			continue
		}

		firstToken := strings.ToUpper(tokens[0])

		// Trata CTEs: WITH name AS (...) SELECT ... -> o verbo real costuma ser o SELECT
		if firstToken == "WITH" {
			// Procura a palavra chave principal após a CTE (SELECT, INSERT, UPDATE, DELETE)
			verbFound := "WITH"
			for _, tok := range tokens[1:] {
				upperTok := strings.ToUpper(tok)
				if upperTok == "SELECT" || upperTok == "INSERT" || upperTok == "UPDATE" || upperTok == "DELETE" {
					verbFound = upperTok
					break
				}
			}
			verbs = append(verbs, verbFound)
		} else {
			verbs = append(verbs, firstToken)
		}
	}

	return verbs
}

// containsForbiddenWord verifica se alguma palavra reservada destrutiva ocorre como token solto na instrução.
func containsForbiddenWord(cleanQuery string, words []string) (bool, string) {
	tokens := strings.FieldsFunc(cleanQuery, func(r rune) bool {
		return unicode.IsSpace(r) || r == ';' || r == '(' || r == ')' || r == ','
	})

	wordMap := make(map[string]bool)
	for _, w := range words {
		wordMap[strings.ToUpper(w)] = true
	}

	for _, t := range tokens {
		upper := strings.ToUpper(t)
		if wordMap[upper] {
			return true, upper
		}
	}
	return false, ""
}

func IsSafeQuery(query string, mode string) CheckResult {
	if mode == "teste" {
		return CheckResult{IsSafe: true, ErrorMsg: ""}
	}

	clean := stripCommentsAndLiterals(query)
	verbs := extractStatementVerbs(clean)

	if len(verbs) == 0 {
		return CheckResult{IsSafe: true, ErrorMsg: ""}
	}

	if mode == "readonly" {
		allowedVerbs := map[string]bool{
			"SELECT":   true,
			"WITH":     true,
			"SHOW":     true,
			"EXPLAIN":  true,
			"DESC":     true,
			"DESCRIBE": true,
		}

		for _, v := range verbs {
			if !allowedVerbs[v] {
				return CheckResult{
					IsSafe:   false,
					ErrorMsg: "Conexão em modo 'readonly'. Apenas consultas de leitura são permitidas.",
				}
			}
		}

		// Checagem extra de segurança contra comandos mutativos em subqueries/instruções escondidas
		forbiddenInReadonly := []string{"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "TRUNCATE", "CREATE", "GRANT", "REVOKE", "EXEC", "EXECUTE"}
		if found, word := containsForbiddenWord(clean, forbiddenInReadonly); found {
			return CheckResult{
				IsSafe:   false,
				ErrorMsg: fmt.Sprintf("Conexão em modo 'readonly'. Operação '%s' não é permitida.", word),
			}
		}

		return CheckResult{IsSafe: true, ErrorMsg: ""}
	}

	// Modo "normal": Bloqueia DROP, DELETE e TRUNCATE
	destructiveVerbs := []string{"DROP", "DELETE", "TRUNCATE"}
	for _, v := range verbs {
		for _, d := range destructiveVerbs {
			if v == d {
				return CheckResult{
					IsSafe:   false,
					ErrorMsg: "Operações destrutivas (DROP, DELETE, TRUNCATE) não são permitidas.",
				}
			}
		}
	}

	if found, word := containsForbiddenWord(clean, destructiveVerbs); found {
		return CheckResult{
			IsSafe:   false,
			ErrorMsg: fmt.Sprintf("Operações destrutivas (%s) não são permitidas.", word),
		}
	}

	return CheckResult{IsSafe: true, ErrorMsg: ""}
}
