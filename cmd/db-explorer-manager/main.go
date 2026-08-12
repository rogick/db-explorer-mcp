package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/rogick/db-explorer-mcp/pkg/config"
	"github.com/rogick/db-explorer-mcp/pkg/db"
)

var reader = bufio.NewReader(os.Stdin)

func ask(prompt string, defaultValue string) string {
	if defaultValue != "" {
		fmt.Printf("%s [%s]: ", prompt, defaultValue)
	} else {
		fmt.Printf("%s: ", prompt)
	}
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultValue
	}
	return input
}

func askPassword(prompt string) string {
	fmt.Printf("%s: ", prompt)
	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		// Fallback se não for TTY
		input, _ := reader.ReadString('\n')
		return strings.TrimSpace(input)
	}
	return strings.TrimSpace(string(bytePassword))
}

func testConnectionAndPromptSave(connDetails config.ConnectionDetails) bool {
	fmt.Println("\nTestando a conexão...")
	executor := db.NewExecutor()
	sqlDB, _, err := executor.OpenConnection(connDetails)
	if err != nil {
		fmt.Printf("❌ Falha na conexão: %v\n", err)
		ans := ask("\nDeseja salvar a conexão assim mesmo? (s/n)", "n")
		return strings.ToLower(ans) == "s"
	}
	sqlDB.Close()
	fmt.Println("✅ Conexão bem-sucedida!")
	return true
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "db-explorer-manager",
		Short: "CLI para gerenciar conexões do DB Explorer MCP",
	}

	// add-oracle
	var oracleUser, oraclePass, oracleDSN, oracleMode string
	cmdAddOracle := &cobra.Command{
		Use:   "add-oracle [alias]",
		Short: "Adicionar uma conexão Oracle",
		Run: func(cmd *cobra.Command, args []string) {
			alias := ""
			if len(args) > 0 {
				alias = args[0]
			}
			for alias == "" {
				alias = ask("Alias (nome da conexão)", "")
			}

			user := oracleUser
			if user == "" {
				user = ask("Usuário", "system")
			}

			password := oraclePass
			for password == "" {
				password = askPassword("Senha")
			}

			dsn := oracleDSN
			if dsn == "" {
				choice := ask("Como deseja informar o endereço? (1) Host/Porta/SID ou (2) DSN completo?", "1")
				if choice == "2" {
					for dsn == "" {
						dsn = ask("DSN completo (ex: localhost:1521/XE)", "")
					}
				} else {
					host := ask("Host", "localhost")
					portStr := ask("Porta", "1521")
					sid := ask("SID / Service Name", "XE")
					dsn = fmt.Sprintf("%s:%s/%s", host, portStr, sid)
				}
			}

			mode := oracleMode
			for mode == "" || (mode != "readonly" && mode != "normal" && mode != "teste") {
				mode = ask("Modo (readonly, normal, teste)", "normal")
			}

			if err := config.ValidateModeHost(mode, dsn); err != nil {
				fmt.Printf("\n❌ %v\n", err)
				os.Exit(1)
			}

			connDetails := config.ConnectionDetails{
				Type:     "oracle",
				Mode:     mode,
				User:     user,
				Password: password,
				DSN:      dsn,
			}

			if !testConnectionAndPromptSave(connDetails) {
				fmt.Println("Operação cancelada.")
				os.Exit(0)
			}

			cfg, _ := config.LoadConfig()
			cfg.Connections[alias] = connDetails
			if err := config.SaveConfig(cfg); err != nil {
				fmt.Printf("❌ Erro ao salvar conexão: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✅ Conexão Oracle '%s' salva com sucesso!\n", alias)
		},
	}
	cmdAddOracle.Flags().StringVarP(&oracleUser, "user", "u", "", "Usuário do banco")
	cmdAddOracle.Flags().StringVarP(&oraclePass, "password", "p", "", "Senha do banco")
	cmdAddOracle.Flags().StringVarP(&oracleDSN, "dsn", "d", "", "String de conexão DSN")
	cmdAddOracle.Flags().StringVarP(&oracleMode, "mode", "m", "", "Modo de acesso: readonly, normal ou teste")

	// add-sqlserver
	var sqlHost, sqlPort, sqlInstance, sqlDatabase, sqlUser, sqlPass, sqlMode string
	cmdAddSqlServer := &cobra.Command{
		Use:   "add-sqlserver [alias]",
		Short: "Adicionar uma conexão SQL Server",
		Run: func(cmd *cobra.Command, args []string) {
			alias := ""
			if len(args) > 0 {
				alias = args[0]
			}
			for alias == "" {
				alias = ask("Alias (nome da conexão)", "")
			}

			host := sqlHost
			if host == "" {
				host = ask("Host", "localhost")
			}

			portStr := sqlPort
			if portStr == "" {
				portStr = ask("Porta [1433] (deixe em branco para o padrão)", "1433")
			}
			port, _ := strconv.Atoi(portStr)

			instance := sqlInstance
			if instance == "" && !cmd.Flags().Changed("instance") {
				instance = ask("Instância (ex: SQLEXPRESS, deixe em branco se não usar)", "")
			}

			database := sqlDatabase
			for database == "" {
				database = ask("Database", "")
			}

			user := sqlUser
			if user == "" {
				user = ask("Usuário", "sa")
			}

			password := sqlPass
			for password == "" {
				password = askPassword("Senha")
			}

			mode := sqlMode
			for mode == "" || (mode != "readonly" && mode != "normal" && mode != "teste") {
				mode = ask("Modo (readonly, normal, teste)", "normal")
			}

			if err := config.ValidateModeHost(mode, host); err != nil {
				fmt.Printf("\n❌ %v\n", err)
				os.Exit(1)
			}

			connDetails := config.ConnectionDetails{
				Type:     "sqlserver",
				Mode:     mode,
				Host:     host,
				Port:     port,
				Instance: instance,
				Database: database,
				User:     user,
				Password: password,
			}

			if !testConnectionAndPromptSave(connDetails) {
				fmt.Println("Operação cancelada.")
				os.Exit(0)
			}

			cfg, _ := config.LoadConfig()
			cfg.Connections[alias] = connDetails
			if err := config.SaveConfig(cfg); err != nil {
				fmt.Printf("❌ Erro ao salvar conexão: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✅ Conexão SQL Server '%s' salva com sucesso!\n", alias)
		},
	}
	cmdAddSqlServer.Flags().StringVarP(&sqlHost, "host", "H", "", "Host do banco")
	cmdAddSqlServer.Flags().StringVarP(&sqlPort, "port", "p", "", "Porta do banco")
	cmdAddSqlServer.Flags().StringVarP(&sqlInstance, "instance", "i", "", "Instância do banco")
	cmdAddSqlServer.Flags().StringVarP(&sqlDatabase, "database", "d", "", "Nome do banco")
	cmdAddSqlServer.Flags().StringVarP(&sqlUser, "user", "u", "", "Usuário do banco")
	cmdAddSqlServer.Flags().StringVarP(&sqlPass, "password", "P", "", "Senha do banco")
	cmdAddSqlServer.Flags().StringVarP(&sqlMode, "mode", "m", "", "Modo de acesso")

	// add-postgres
	var pgHost, pgPort, pgDatabase, pgUser, pgPass, pgMode string
	cmdAddPostgres := &cobra.Command{
		Use:   "add-postgres [alias]",
		Short: "Adicionar uma conexão PostgreSQL",
		Run: func(cmd *cobra.Command, args []string) {
			alias := ""
			if len(args) > 0 {
				alias = args[0]
			}
			for alias == "" {
				alias = ask("Alias (nome da conexão)", "")
			}

			host := pgHost
			if host == "" {
				host = ask("Host", "localhost")
			}

			portStr := pgPort
			if portStr == "" {
				portStr = ask("Porta", "5432")
			}
			port, _ := strconv.Atoi(portStr)

			database := pgDatabase
			if database == "" {
				database = ask("Database", "postgres")
			}

			user := pgUser
			if user == "" {
				user = ask("Usuário", "postgres")
			}

			password := pgPass
			for password == "" {
				password = askPassword("Senha")
			}

			mode := pgMode
			for mode == "" || (mode != "readonly" && mode != "normal" && mode != "teste") {
				mode = ask("Modo (readonly, normal, teste)", "normal")
			}

			if err := config.ValidateModeHost(mode, host); err != nil {
				fmt.Printf("\n❌ %v\n", err)
				os.Exit(1)
			}

			connDetails := config.ConnectionDetails{
				Type:     "postgres",
				Mode:     mode,
				Host:     host,
				Port:     port,
				Database: database,
				User:     user,
				Password: password,
			}

			if !testConnectionAndPromptSave(connDetails) {
				fmt.Println("Operação cancelada.")
				os.Exit(0)
			}

			cfg, _ := config.LoadConfig()
			cfg.Connections[alias] = connDetails
			if err := config.SaveConfig(cfg); err != nil {
				fmt.Printf("❌ Erro ao salvar conexão: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✅ Conexão PostgreSQL '%s' salva com sucesso!\n", alias)
		},
	}
	cmdAddPostgres.Flags().StringVarP(&pgHost, "host", "H", "", "Host do banco")
	cmdAddPostgres.Flags().StringVarP(&pgPort, "port", "p", "", "Porta do banco")
	cmdAddPostgres.Flags().StringVarP(&pgDatabase, "database", "d", "", "Nome do banco")
	cmdAddPostgres.Flags().StringVarP(&pgUser, "user", "u", "", "Usuário do banco")
	cmdAddPostgres.Flags().StringVarP(&pgPass, "password", "P", "", "Senha do banco")
	cmdAddPostgres.Flags().StringVarP(&pgMode, "mode", "m", "", "Modo de acesso")

	// add-mysql
	var myHost, myPort, myDatabase, myUser, myPass, myMode string
	cmdAddMysql := &cobra.Command{
		Use:   "add-mysql [alias]",
		Short: "Adicionar uma conexão MySQL",
		Run: func(cmd *cobra.Command, args []string) {
			alias := ""
			if len(args) > 0 {
				alias = args[0]
			}
			for alias == "" {
				alias = ask("Alias (nome da conexão)", "")
			}

			host := myHost
			if host == "" {
				host = ask("Host", "localhost")
			}

			portStr := myPort
			if portStr == "" {
				portStr = ask("Porta", "3306")
			}
			port, _ := strconv.Atoi(portStr)

			database := myDatabase
			if database == "" {
				database = ask("Database", "mysql")
			}

			user := myUser
			if user == "" {
				user = ask("Usuário", "root")
			}

			password := myPass
			for password == "" {
				password = askPassword("Senha")
			}

			mode := myMode
			for mode == "" || (mode != "readonly" && mode != "normal" && mode != "teste") {
				mode = ask("Modo (readonly, normal, teste)", "normal")
			}

			if err := config.ValidateModeHost(mode, host); err != nil {
				fmt.Printf("\n❌ %v\n", err)
				os.Exit(1)
			}

			connDetails := config.ConnectionDetails{
				Type:     "mysql",
				Mode:     mode,
				Host:     host,
				Port:     port,
				Database: database,
				User:     user,
				Password: password,
			}

			if !testConnectionAndPromptSave(connDetails) {
				fmt.Println("Operação cancelada.")
				os.Exit(0)
			}

			cfg, _ := config.LoadConfig()
			cfg.Connections[alias] = connDetails
			if err := config.SaveConfig(cfg); err != nil {
				fmt.Printf("❌ Erro ao salvar conexão: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✅ Conexão MySQL '%s' salva com sucesso!\n", alias)
		},
	}
	cmdAddMysql.Flags().StringVarP(&myHost, "host", "H", "", "Host do banco")
	cmdAddMysql.Flags().StringVarP(&myPort, "port", "p", "", "Porta do banco")
	cmdAddMysql.Flags().StringVarP(&myDatabase, "database", "d", "", "Nome do banco")
	cmdAddMysql.Flags().StringVarP(&myUser, "user", "u", "", "Usuário do banco")
	cmdAddMysql.Flags().StringVarP(&myPass, "password", "P", "", "Senha do banco")
	cmdAddMysql.Flags().StringVarP(&myMode, "mode", "m", "", "Modo de acesso")

	// edit <alias>
	cmdEdit := &cobra.Command{
		Use:   "edit [alias]",
		Short: "Editar uma conexão existente",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			alias := args[0]
			cfg, err := config.LoadConfig()
			if err != nil {
				fmt.Printf("❌ Erro ao carregar configurações: %v\n", err)
				os.Exit(1)
			}

			conn, exists := cfg.Connections[alias]
			if !exists {
				fmt.Printf("❌ Conexão '%s' não encontrada.\n", alias)
				os.Exit(1)
			}

			fmt.Printf("Editando conexão '%s' do tipo '%s'\n", alias, conn.Type)
			fmt.Println("Pressione Enter para manter o valor atual.")

			if conn.Type == "oracle" {
				conn.User = ask("Usuário", conn.User)
				passPrompt := "Senha"
				if conn.Password != "" {
					passPrompt = "Senha [***]"
				}
				pass := askPassword(passPrompt)
				if pass != "" {
					conn.Password = pass
				}
				conn.DSN = ask("DSN", conn.DSN)
			} else if conn.Type == "sqlserver" {
				curHost := conn.Host
				if curHost == "" && conn.Server != "" {
					parts := strings.Split(conn.Server, ":")
					curHost = parts[0]
				}
				if curHost == "" {
					curHost = "localhost"
				}

				conn.Host = ask("Host", curHost)
				portStr := ask("Porta", fmt.Sprintf("%d", conn.Port))
				conn.Port, _ = strconv.Atoi(portStr)
				conn.Instance = ask("Instância", conn.Instance)
				conn.Database = ask("Database", conn.Database)
				conn.User = ask("Usuário", conn.User)

				passPrompt := "Senha"
				if conn.Password != "" {
					passPrompt = "Senha [***]"
				}
				pass := askPassword(passPrompt)
				if pass != "" {
					conn.Password = pass
				}
			} else { // postgres or mysql
				conn.Host = ask("Host", conn.Host)
				portStr := ask("Porta", fmt.Sprintf("%d", conn.Port))
				conn.Port, _ = strconv.Atoi(portStr)
				conn.Database = ask("Database", conn.Database)
				conn.User = ask("Usuário", conn.User)

				passPrompt := "Senha"
				if conn.Password != "" {
					passPrompt = "Senha [***]"
				}
				pass := askPassword(passPrompt)
				if pass != "" {
					conn.Password = pass
				}
			}

			curMode := conn.Mode
			if curMode == "" {
				curMode = "normal"
			}
			conn.Mode = ask("Modo (readonly, normal, teste)", curMode)

			addr := conn.DSN
			if addr == "" {
				addr = conn.Host
			}

			if err := config.ValidateModeHost(conn.Mode, addr); err != nil {
				fmt.Printf("\n❌ %v\n", err)
				os.Exit(1)
			}

			if !testConnectionAndPromptSave(conn) {
				fmt.Println("Operação cancelada.")
				os.Exit(0)
			}

			cfg.Connections[alias] = conn
			if err := config.SaveConfig(cfg); err != nil {
				fmt.Printf("❌ Erro ao salvar conexão: %v\n", err)
				os.Exit(1)
			}

			fmt.Printf("✅ Conexão '%s' atualizada com sucesso!\n", alias)
		},
	}

	// list
	cmdList := &cobra.Command{
		Use:   "list",
		Short: "Listar as conexões configuradas",
		Run: func(cmd *cobra.Command, args []string) {
			cfg, err := config.LoadConfig()
			if err != nil {
				fmt.Printf("❌ Erro ao carregar configurações: %v\n", err)
				os.Exit(1)
			}

			if len(cfg.Connections) == 0 {
				fmt.Println("Nenhuma conexão configurada.")
			} else {
				fmt.Println("Conexões configuradas:")
				for alias, c := range cfg.Connections {
					mode := c.Mode
					if mode == "" {
						mode = "normal"
					}
					fmt.Printf(" - %s (%s) [Modo: %s]\n", alias, c.Type, mode)
				}
			}
		},
	}

	// remove <alias>
	cmdRemove := &cobra.Command{
		Use:   "remove [alias]",
		Short: "Remover uma conexão",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			alias := args[0]
			cfg, err := config.LoadConfig()
			if err != nil {
				fmt.Printf("❌ Erro ao carregar configurações: %v\n", err)
				os.Exit(1)
			}

			if _, exists := cfg.Connections[alias]; exists {
				delete(cfg.Connections, alias)
				if err := config.SaveConfig(cfg); err != nil {
					fmt.Printf("❌ Erro ao remover conexão: %v\n", err)
					os.Exit(1)
				}
				fmt.Printf("✅ Conexão '%s' removida.\n", alias)
			} else {
				fmt.Printf("❌ Conexão '%s' não encontrada.\n", alias)
			}
		},
	}

	rootCmd.AddCommand(cmdAddOracle, cmdAddSqlServer, cmdAddPostgres, cmdAddMysql, cmdEdit, cmdList, cmdRemove)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
