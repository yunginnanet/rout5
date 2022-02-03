package config

import (
	"fmt"
	"os"

	"github.com/rs/zerolog"
)

func setDefaults() {
	var (
		configSections = []string{"logger", "http", "interfaces"}
		deflogdir      = "/var/logging/" + Title
	)

	const defNoColor = false

	Opt := make(map[string]map[string]interface{})

	Opt["logger"] = map[string]interface{}{
		"debug":             true,
		"trace":             false,
		"directory":         deflogdir,
		"nocolor":           defNoColor,
		"use_date_filename": true,
	}

	Opt["admin"] = map[string]interface{}{
		"ssh_bind": "0.0.0.0:2222",
		"web_bind": "0.0.0.0:8080",
	}

	Opt["dhcp"] = map[string]interface{}{
		"lease_time_seconds": 3600,
	}

	Opt["interfaces"] = map[string]interface{}{
		"wan_ifnames": []string{"eth0"},
		"lan_ifnames": []string{"eth1"},
	}

	for _, def := range configSections {
		Snek.SetDefault(def, Opt[def])
	}

	if GenConfig {
		if err = Snek.SafeWriteConfigAs("./config.toml"); err != nil {
			fmt.Println(err.Error())
			os.Exit(1)
		}
		println("config generated -> ./config.toml")
		os.Exit(0)
	}

}

func processOpts() {
	// string slice options and their exported variables
	stringOpt := map[string]*[]string{
		"interfaces.wan_ifnames": &PreferredWAN,
		"interfaces.lan_ifnames": &PreferredLAN,
		"admin.web_bind":         &AdminHTTP,
		"admin.ssh_bind":         &AdminSSH,
	}

	// bool options and their exported variables
	boolOpt := map[string]*bool{
		"logger.nocolor": &NoColor,
		"logger.debug":   &Debug,
		"logger.trace":   &Trace,
	}

	// int options and their exported variables
	intOpt := map[string]*int{}

	for key, opt := range intOpt {
		*opt = Snek.GetInt(key)
	}

	for key, opt := range stringOpt {
		*opt = Snek.GetStringSlice(key)
	}
	for key, opt := range boolOpt {
		*opt = Snek.GetBool(key)
	}
}

func associateExportedVariables() {
	processOpts()
	switch {
	case Debug:
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case Trace:
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
}
