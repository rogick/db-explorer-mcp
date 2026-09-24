package config

import (
	"sort"
	"strings"
)

// ConnectionMatch representa o resultado de uma busca por conexão com score de relevância.
type ConnectionMatch struct {
	Alias      string            `json:"alias"`
	Connection ConnectionDetails `json:"connection"`
	Score      int               `json:"score"`
}

// FindConnections busca conexões cadastradas por nome exato ou aproximado.
// A lista de resultados é ordenada por relevância (menor score = maior prioridade).
func (c *Config) FindConnections(query string) []ConnectionMatch {
	if c.Connections == nil || len(c.Connections) == 0 {
		return nil
	}

	qTrim := strings.TrimSpace(query)
	if qTrim == "" {
		var all []ConnectionMatch
		for k, conn := range c.Connections {
			all = append(all, ConnectionMatch{Alias: k, Connection: conn, Score: 0})
		}
		sort.Slice(all, func(i, j int) bool {
			return all[i].Alias < all[j].Alias
		})
		return all
	}

	qLower := strings.ToLower(qTrim)
	qNorm := normalizeString(qTrim)
	qTokens := splitTokens(qTrim)

	var matches []ConnectionMatch

	for alias, conn := range c.Connections {
		bestScore := -1

		// 1. Correspondência exata (case-sensitive)
		if alias == qTrim {
			bestScore = 0
		} else if strings.ToLower(alias) == qLower {
			// 2. Correspondência exata (case-insensitive)
			bestScore = 1
		}

		aLower := strings.ToLower(alias)
		aNorm := normalizeString(alias)
		aTokens := splitTokens(alias)

		if bestScore < 0 {
			// 3. Prefixo no alias
			if strings.HasPrefix(aLower, qLower) {
				bestScore = 2
			} else if strings.HasSuffix(aLower, qLower) {
				// 4. Sufixo no alias
				bestScore = 3
			}
		}

		// 5. Comparação de tokens (palavras separadas por _, -, ., espaços)
		if bestScore < 0 || bestScore > 4 {
			for _, token := range aTokens {
				if token == qLower {
					if bestScore < 0 || bestScore > 4 {
						bestScore = 4
					}
					break
				}
			}
		}

		if bestScore < 0 || bestScore > 5 {
			for _, token := range aTokens {
				if strings.HasPrefix(token, qLower) {
					if bestScore < 0 || bestScore > 5 {
						bestScore = 5
					}
					break
				}
			}
		}

		// 6. Substring simples
		if (bestScore < 0 || bestScore > 6) && strings.Contains(aLower, qLower) {
			bestScore = 6
		}

		// 7. Normalizado (remove separadores _, -, .)
		if (bestScore < 0 || bestScore > 7) && qNorm != "" {
			if aNorm == qNorm {
				bestScore = 7
			} else if strings.HasPrefix(aNorm, qNorm) {
				if bestScore < 0 || bestScore > 8 {
					bestScore = 8
				}
			} else if strings.Contains(aNorm, qNorm) {
				if bestScore < 0 || bestScore > 9 {
					bestScore = 9
				}
			}
		}

		// 8. Se todos os tokens da query estiverem contidos nos tokens do alias
		if (bestScore < 0 || bestScore > 10) && len(qTokens) > 1 {
			allTokensFound := true
			for _, qt := range qTokens {
				found := false
				for _, at := range aTokens {
					if strings.Contains(at, qt) {
						found = true
						break
					}
				}
				if !found {
					allTokensFound = false
					break
				}
			}
			if allTokensFound {
				bestScore = 10
			}
		}

		// 9. Correspondência fuzzy / tolerância a erros de digitação (Levenshtein)
		if bestScore < 0 && len(qLower) >= 3 {
			maxDist := 1
			if len(qLower) >= 6 {
				maxDist = 2
			}

			// Comparação entre a query inteira e tokens do alias
			for _, token := range aTokens {
				if len(token) >= 3 {
					dist := levenshtein(qLower, token)
					if dist <= maxDist {
						score := 20 + dist
						if bestScore < 0 || score < bestScore {
							bestScore = score
						}
					}
				}
			}

			// Comparação entre a query inteira e o alias inteiro
			distFull := levenshtein(qLower, aLower)
			if distFull <= maxDist {
				score := 30 + distFull
				if bestScore < 0 || score < bestScore {
					bestScore = score
				}
			}
		}

		if bestScore >= 0 {
			matches = append(matches, ConnectionMatch{
				Alias:      alias,
				Connection: conn,
				Score:      bestScore,
			})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score < matches[j].Score
		}
		if len(matches[i].Alias) != len(matches[j].Alias) {
			return len(matches[i].Alias) < len(matches[j].Alias)
		}
		return matches[i].Alias < matches[j].Alias
	})

	return matches
}

func normalizeString(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func splitTokens(s string) []string {
	var tokens []string
	var current []rune
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			current = append(current, r)
		} else if len(current) > 0 {
			tokens = append(tokens, string(current))
			current = nil
		}
	}
	if len(current) > 0 {
		tokens = append(tokens, string(current))
	}
	return tokens
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	dp := make([][]int, la+1)
	for i := range dp {
		dp[i] = make([]int, lb+1)
		dp[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		dp[0][j] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			del := dp[i-1][j] + 1
			ins := dp[i][j-1] + 1
			sub := dp[i-1][j-1] + cost
			minVal := del
			if ins < minVal {
				minVal = ins
			}
			if sub < minVal {
				minVal = sub
			}
			dp[i][j] = minVal
		}
	}
	return dp[la][lb]
}
