package cmd

import (
	"strings"

	"github.com/kat-co/vala"
	"github.com/nullbio/abcweb/config"
	"github.com/spf13/cobra"
	"github.com/vattle/sqlboiler/bdb/drivers"
	"github.com/vattle/sqlboiler/boilingcore"
)

var modelsCmdConfig *boilingcore.Config
var modelsCmdState *boilingcore.State

var migrationCmdConfig migrateConfig

// generateCmd represents the "generate" command
var generateCmd = &cobra.Command{
	Use:     "generate",
	Short:   "Generate your database models and migration files",
	Example: "abcweb generate models\nabcweb generate migration add_users",
}

// modelsCmd represents the "generate models" command
var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Generate your database models",
	Long: `Generate models will connect to your database and generate your models from your existing database structure. 
Don't forget to run your migrations.

This tool pipes out to SQLBoiler: https://github.com/vattle/sqlboiler -- See README.md at sqlboiler repo for API guidance.`,
	Example: "abcweb generate models",
	PreRunE: modelsCmdPreRun,
	RunE:    modelsCmdRun,
}

// migrationCmd represents the "generate migration" command
var migrationCmd = &cobra.Command{
	Use:   "migration <name> [flags]",
	Short: "Generate a migration file",
	Long: `Generate migration will generate a .go or .sql migration file in your migrations directory.
This tool pipes out to Goose: https://github.com/pressly/goose`,
	Example: "abcweb generate migration add_users",
	PreRun:  migrationCmdPreRun,
	RunE:    migrationCmdRun,
}

// GenerateInit initializes the build commands and flags
func GenerateInit() {
	// models flags
	modelsCmd.Flags().StringP("db", "", "", `Valid options: postgres|mysql (default "database.toml db field")`)
	modelsCmd.Flags().StringP("output", "o", "models", "The name of the folder to output to")
	modelsCmd.Flags().StringP("schema", "s", "public", "The name of your database schema, for databases that support real schemas")
	modelsCmd.Flags().StringP("pkgname", "p", "models", "The name you wish to assign to your generated package")
	modelsCmd.Flags().StringP("basedir", "", "", "The base directory has the templates and templates_test folders")
	modelsCmd.Flags().StringSliceP("blacklist", "b", nil, "Do not include these tables in your generated package")
	modelsCmd.Flags().StringSliceP("whitelist", "w", nil, "Only include these tables in your generated package")
	modelsCmd.Flags().StringSliceP("tag", "t", nil, "Struct tags to be included on your models in addition to json, yaml, toml")
	modelsCmd.Flags().BoolP("debug", "d", false, "Debug mode prints stack traces on error")
	modelsCmd.Flags().BoolP("no-tests", "", false, "Disable generated go test files")
	modelsCmd.Flags().BoolP("no-hooks", "", false, "Disable hooks feature for your models")
	modelsCmd.Flags().BoolP("no-auto-timestamps", "", false, "Disable automatic timestamps for created_at/updated_at")
	modelsCmd.Flags().BoolP("tinyint-not-bool", "", false, "Map MySQL tinyint(1) in Go to int8 instead of bool")

	// migration flags
	migrationCmd.Flags().BoolP("sql", "s", false, "Generate an .sql migration instead of a .go migration")
	migrationCmd.Flags().StringP("dir", "d", migrationsDirectory, "Directory with migration files")

	RootCmd.AddCommand(generateCmd)

	// Add generate subcommands
	generateCmd.AddCommand(modelsCmd)
	generateCmd.AddCommand(migrationCmd)

	config.ModeViper.BindPFlags(modelsCmd.Flags())
	config.ModeViper.BindPFlags(migrationCmd.Flags())
	config.ModeViper.BindPFlags(generateCmd.Flags())
}

// modelsCmdPreRun sets up the modelsCmdState and modelsCmdConfig objects
func modelsCmdPreRun(cmd *cobra.Command, args []string) error {
	var err error

	modelsCmdConfig = &boilingcore.Config{
		DriverName:       config.ModeViper.GetString("db"),
		OutFolder:        config.ModeViper.GetString("output"),
		Schema:           config.ModeViper.GetString("schema"),
		PkgName:          config.ModeViper.GetString("pkgname"),
		BaseDir:          config.ModeViper.GetString("basedir"),
		Debug:            config.ModeViper.GetBool("debug"),
		NoTests:          config.ModeViper.GetBool("no-tests"),
		NoHooks:          config.ModeViper.GetBool("no-hooks"),
		NoAutoTimestamps: config.ModeViper.GetBool("no-auto-timestamps"),
	}

	// BUG: https://github.com/spf13/viper/pull/296
	// Look up the value of blacklist, whitelist & tags directly from PFlags in Cobra if we
	// detect a malformed value coming out of viper.
	modelsCmdConfig.BlacklistTables = config.ModeViper.GetStringSlice("blacklist")
	if len(modelsCmdConfig.BlacklistTables) == 1 && strings.ContainsRune(modelsCmdConfig.BlacklistTables[0], ',') {
		modelsCmdConfig.BlacklistTables, err = cmd.Flags().GetStringSlice("blacklist")
		if err != nil {
			return err
		}
	}

	modelsCmdConfig.WhitelistTables = config.ModeViper.GetStringSlice("whitelist")
	if len(modelsCmdConfig.WhitelistTables) == 1 && strings.ContainsRune(modelsCmdConfig.WhitelistTables[0], ',') {
		modelsCmdConfig.WhitelistTables, err = cmd.Flags().GetStringSlice("whitelist")
		if err != nil {
			return err
		}
	}

	modelsCmdConfig.Tags = config.ModeViper.GetStringSlice("tag")
	if len(modelsCmdConfig.Tags) == 1 && strings.ContainsRune(modelsCmdConfig.Tags[0], ',') {
		modelsCmdConfig.Tags, err = cmd.Flags().GetStringSlice("tag")
		if err != nil {
			return err
		}
	}

	if modelsCmdConfig.DriverName == "postgres" {
		modelsCmdConfig.Postgres = boilingcore.PostgresConfig{
			User:    config.ModeViper.GetString("user"),
			Pass:    config.ModeViper.GetString("pass"),
			Host:    config.ModeViper.GetString("host"),
			Port:    config.ModeViper.GetInt("port"),
			DBName:  config.ModeViper.GetString("dbname"),
			SSLMode: config.ModeViper.GetString("sslmode"),
		}

		if modelsCmdConfig.Postgres.SSLMode == "" {
			modelsCmdConfig.Postgres.SSLMode = "require"
			config.ModeViper.Set("sslmode", modelsCmdConfig.Postgres.SSLMode)
		}

		if modelsCmdConfig.Postgres.Port == 0 {
			modelsCmdConfig.Postgres.Port = 5432
			config.ModeViper.Set("port", modelsCmdConfig.Postgres.Port)
		}

		err = vala.BeginValidation().Validate(
			vala.StringNotEmpty(modelsCmdConfig.Postgres.User, "user"),
			vala.StringNotEmpty(modelsCmdConfig.Postgres.Host, "host"),
			vala.Not(vala.Equals(modelsCmdConfig.Postgres.Port, 0, "port")),
			vala.StringNotEmpty(modelsCmdConfig.Postgres.DBName, "dbname"),
			vala.StringNotEmpty(modelsCmdConfig.Postgres.SSLMode, "sslmode"),
		).Check()

		if err != nil {
			return err
		}
	}

	if modelsCmdConfig.DriverName == "mysql" {
		modelsCmdConfig.MySQL = boilingcore.MySQLConfig{
			User:    config.ModeViper.GetString("user"),
			Pass:    config.ModeViper.GetString("pass"),
			Host:    config.ModeViper.GetString("host"),
			Port:    config.ModeViper.GetInt("port"),
			DBName:  config.ModeViper.GetString("dbname"),
			SSLMode: config.ModeViper.GetString("sslmode"),
		}

		// Set MySQL TinyintAsBool global var. This flag only applies to MySQL.
		// Invert the value since ABCWeb takes it as "not" bool instead of "as" bool.
		drivers.TinyintAsBool = !config.ModeViper.GetBool("tinyint-not-bool")

		// MySQL doesn't have schemas, just databases
		modelsCmdConfig.Schema = modelsCmdConfig.MySQL.DBName

		if modelsCmdConfig.MySQL.SSLMode == "" {
			modelsCmdConfig.MySQL.SSLMode = "true"
			config.ModeViper.Set("sslmode", modelsCmdConfig.MySQL.SSLMode)
		}

		if modelsCmdConfig.MySQL.Port == 0 {
			modelsCmdConfig.MySQL.Port = 3306
			config.ModeViper.Set("port", modelsCmdConfig.MySQL.Port)
		}

		err = vala.BeginValidation().Validate(
			vala.StringNotEmpty(modelsCmdConfig.MySQL.User, "user"),
			vala.StringNotEmpty(modelsCmdConfig.MySQL.Host, "host"),
			vala.Not(vala.Equals(modelsCmdConfig.MySQL.Port, 0, "port")),
			vala.StringNotEmpty(modelsCmdConfig.MySQL.DBName, "dbname"),
			vala.StringNotEmpty(modelsCmdConfig.MySQL.SSLMode, "sslmode"),
		).Check()

		if err != nil {
			return err
		}
	}

	modelsCmdState, err = boilingcore.New(modelsCmdConfig)
	return err
}

func modelsCmdRun(cmd *cobra.Command, args []string) error {
	return modelsCmdState.Run(true)
}

func migrationCmdPreRun(cmd *cobra.Command, args []string) {
	migrationCmdConfig = migrateConfig{
		SQL: config.ModeViper.GetBool("sql"),
		Dir: config.ModeViper.GetString("dir"),
	}
}

func migrationCmdRun(cmd *cobra.Command, args []string) error {
	return nil
}
