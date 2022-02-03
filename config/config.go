package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/spf13/viper"
)

func init() {
	prefConfigLocation = "/etc/" + Title
	Snek = viper.New()
}

func writeConfig() {
	if _, err := os.Stat(prefConfigLocation); os.IsNotExist(err) {
		if err = os.MkdirAll(prefConfigLocation, 0o755); err != nil {
			println("error writing new config: " + err.Error())
			os.Exit(1)
		}
	}

	newconfig := prefConfigLocation + "/" + "config.toml"
	if err = Snek.SafeWriteConfigAs(newconfig); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	Filename = newconfig
}

// Init will initialize our toml configuration engine and define our default configuration values which can be written to a new configuration file if desired
func Init() {
	Snek.SetConfigType("toml")
	Snek.SetConfigName("config")

	argParse()

	if customconfig {
		associateExportedVariables()
		return
	}

	setConfigFileLocations()
	setDefaults()

	for _, loc := range configLocations {
		Snek.AddConfigPath(loc)
	}

	if err = Snek.MergeInConfig(); err != nil {
		writeConfig()
	}

	if len(Filename) < 1 {
		Filename = Snek.ConfigFileUsed()
	}

	associateExportedVariables()
}

func setConfigFileLocations() {
	configLocations = append(configLocations, "./")

	if runtime.GOOS != "windows" {
		configLocations = append(configLocations,
			prefConfigLocation,
			"/etc/"+Title+"/",
			"../", "../../")
	}
}

func loadCustomConfig(path string) {
	if f, err = os.Open(path); err != nil {
		println("Error opening specified config file: " + path)
		panic("config file open fatal error: " + err.Error())
	}
	buf, err := io.ReadAll(f)
	err2 := Snek.ReadConfig(bytes.NewBuffer(buf))
	switch {
	case err != nil:
		fmt.Println("config file read fatal error: ", err.Error())
	case err2 != nil:
		fmt.Println("config file read fatal error: ", err2.Error())
	default:
		break
	}
	customconfig = true
}

func printUsage() {
	println("\n" + Title + " v" + Version + " Usage\n")
	println("-c <config.toml> - Specify config file")
	println("--nocolor - disable color and banner ")
	println("--banner - show banner + version and exit")
	println("--genconfig - write default config to 'default.toml' then exit")
	os.Exit(0)
}

// TODO: should probably just make a proper CLI with flags or something
func argParse() {
	for i, arg := range os.Args {
		switch arg {
		case "-h":
			printUsage()
		case "--genconfig":
			GenConfig = true
		case "--config", "-c":
			if len(os.Args) <= i-1 {
				panic("syntax error! expected file after -c")
			}
			loadCustomConfig(os.Args[i+1])
		default:
			continue
		}
	}
}
