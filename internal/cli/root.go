// Package cli builds the developer-facing mei command tree.
package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// Settings contains the resolved CLI configuration.
type Settings struct {
	ConfigFile string
	Repo       string
	Format     string
	Verbose    bool
}

// New creates a configured mei command. Passing nil creates a fresh Viper
// instance, which keeps command execution isolated and easy to test.
func New(config *viper.Viper) *cobra.Command {
	if config == nil {
		config = viper.New()
	}
	settings := &Settings{}

	root := &cobra.Command{
		Use:           "mei",
		Short:         "Meiosis developer CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := readConfig(config); err != nil {
				return err
			}
			settings.ConfigFile = config.ConfigFileUsed()
			settings.Repo = config.GetString("repo")
			settings.Format = config.GetString("format")
			settings.Verbose = config.GetBool("verbose")
			return nil
		},
	}

	config.SetEnvPrefix("MEIOSIS")
	config.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))
	config.AutomaticEnv()
	config.SetDefault("repo", ".")
	config.SetDefault("format", "text")
	config.SetDefault("verbose", false)

	flags := root.PersistentFlags()
	flags.String("config", "", "configuration file (default: .meiosis/config.yaml)")
	flags.String("repo", "", "repository path")
	flags.String("format", "", "output format (text or json)")
	flags.Bool("verbose", false, "enable verbose output")
	bindFlag(config, flags, "config")
	bindFlag(config, flags, "repo")
	bindFlag(config, flags, "format")
	bindFlag(config, flags, "verbose")

	root.AddCommand(
		newActionCommand("intent", "Manage intents", settings),
		newActionCommand("attempt", "Manage attempts", settings),
		newActionCommand("evidence", "Manage evidence", settings),
		newActionCommand("status", "Show repository status", settings),
		newCompletionCommand(),
	)
	return root
}

func bindFlag(config *viper.Viper, flags *pflag.FlagSet, name string) {
	_ = config.BindPFlag(name, flags.Lookup(name))
}

func readConfig(config *viper.Viper) error {
	if file := config.GetString("config"); file != "" {
		config.SetConfigFile(file)
	} else {
		config.SetConfigName("config")
		config.SetConfigType("yaml")
		config.AddConfigPath(".meiosis")
	}
	var notFound viper.ConfigFileNotFoundError
	if err := config.ReadInConfig(); err != nil && !errors.As(err, &notFound) {
		return fmt.Errorf("read config: %w", err)
	}
	return nil
}
