package config

import (
	"testing"
)

func TestFindConnections(t *testing.T) {
	cfg := &Config{
		Connections: map[string]ConnectionDetails{
			"SOFTRH_GUI_REMOTO": {Type: "oracle", Mode: "readonly"},
			"SOFTRH_LOCAL":      {Type: "oracle", Mode: "normal"},
			"modelosql-teste":   {Type: "sqlserver", Mode: "teste"},
			"sankhya_prod":      {Type: "postgres", Mode: "readonly"},
			"dev_mysql":         {Type: "mysql", Mode: "normal"},
		},
	}

	t.Run("Empty query returns all connections sorted alphabetically", func(t *testing.T) {
		matches := cfg.FindConnections("")
		if len(matches) != 5 {
			t.Fatalf("Esperado 5 conexões, obteve %d", len(matches))
		}
		expectedOrder := []string{"SOFTRH_GUI_REMOTO", "SOFTRH_LOCAL", "dev_mysql", "modelosql-teste", "sankhya_prod"}
		for i, exp := range expectedOrder {
			if matches[i].Alias != exp {
				t.Errorf("Posição %d esperada %s, obteve %s", i, exp, matches[i].Alias)
			}
		}
	})

	t.Run("Exact match (case-sensitive)", func(t *testing.T) {
		matches := cfg.FindConnections("SOFTRH_LOCAL")
		if len(matches) == 0 {
			t.Fatalf("Esperado ao menos 1 resultado")
		}
		if matches[0].Alias != "SOFTRH_LOCAL" || matches[0].Score != 0 {
			t.Errorf("Primeiro resultado deveria ser SOFTRH_LOCAL com score 0, obteve %s com score %d", matches[0].Alias, matches[0].Score)
		}
	})

	t.Run("Exact match (case-insensitive)", func(t *testing.T) {
		matches := cfg.FindConnections("softrh_local")
		if len(matches) == 0 {
			t.Fatalf("Esperado ao menos 1 resultado")
		}
		if matches[0].Alias != "SOFTRH_LOCAL" || matches[0].Score != 1 {
			t.Errorf("Primeiro resultado deveria ser SOFTRH_LOCAL com score 1, obteve %s com score %d", matches[0].Alias, matches[0].Score)
		}
	})

	t.Run("Prefix match finds multiple connections", func(t *testing.T) {
		matches := cfg.FindConnections("softrh")
		if len(matches) < 2 {
			t.Fatalf("Esperado ao menos 2 resultados para prefixo 'softrh', obteve %d", len(matches))
		}
		foundRemote := false
		foundLocal := false
		for _, m := range matches {
			if m.Alias == "SOFTRH_GUI_REMOTO" {
				foundRemote = true
			}
			if m.Alias == "SOFTRH_LOCAL" {
				foundLocal = true
			}
		}
		if !foundRemote || !foundLocal {
			t.Errorf("Esperado encontrar SOFTRH_GUI_REMOTO e SOFTRH_LOCAL")
		}
	})

	t.Run("Suffix match", func(t *testing.T) {
		matches := cfg.FindConnections("remoto")
		if len(matches) == 0 {
			t.Fatalf("Esperado encontrar resultado para 'remoto'")
		}
		if matches[0].Alias != "SOFTRH_GUI_REMOTO" {
			t.Errorf("Esperado SOFTRH_GUI_REMOTO, obteve %s", matches[0].Alias)
		}
	})

	t.Run("Substring match", func(t *testing.T) {
		matches := cfg.FindConnections("gui")
		if len(matches) == 0 {
			t.Fatalf("Esperado encontrar resultado para 'gui'")
		}
		if matches[0].Alias != "SOFTRH_GUI_REMOTO" {
			t.Errorf("Esperado SOFTRH_GUI_REMOTO, obteve %s", matches[0].Alias)
		}
	})

	t.Run("Normalized delimiter match (soft-rh)", func(t *testing.T) {
		matches := cfg.FindConnections("soft-rh")
		if len(matches) < 2 {
			t.Fatalf("Esperado encontrar SOFTRH mesmo com hífen, obteve %d resultados", len(matches))
		}
	})

	t.Run("Fuzzy match with typo (sankya -> sankhya_prod)", func(t *testing.T) {
		matches := cfg.FindConnections("sankya")
		if len(matches) == 0 {
			t.Fatalf("Esperado encontrar 'sankhya_prod' para 'sankya'")
		}
		if matches[0].Alias != "sankhya_prod" {
			t.Errorf("Esperado sankhya_prod, obteve %s", matches[0].Alias)
		}
	})

	t.Run("Fuzzy match with typo (my-sql -> dev_mysql)", func(t *testing.T) {
		matches := cfg.FindConnections("mysql")
		if len(matches) == 0 {
			t.Fatalf("Esperado encontrar 'dev_mysql' para 'mysql'")
		}
		if matches[0].Alias != "dev_mysql" {
			t.Errorf("Esperado dev_mysql, obteve %s", matches[0].Alias)
		}
	})

	t.Run("No match returns empty slice", func(t *testing.T) {
		matches := cfg.FindConnections("conexao_totalmente_inexistente_xyz")
		if len(matches) != 0 {
			t.Errorf("Esperado 0 resultados, obteve %d", len(matches))
		}
	})
}
