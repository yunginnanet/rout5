package config

import (
	"net"
	"os"
	"time"

	"github.com/spf13/viper"
)

const (
	// Version roughly represents the applications current version.
	Version = "0.1"
	// Title is the name of the application used throughout the configuration process.
	Title = "Rout5"
)

var (
	// GenConfig when toggled causes HellPot to write its default config to the cwd and then exit.
	GenConfig = false
	// NoColor stops zerolog from outputting color, necessary on Windows.
	NoColor = true
)

// "data"
var (
	DataDirectory string
)

// "interfaces"
var (
	PreferredWAN []string
	PreferredLAN []string
	// Uplinks are connections to WAN(s).
	Uplinks = make(map[string]net.Interface)
	// Downlinks are connections to our LAN(s).
	Downlinks = make(map[string]net.Interface)
)

// "dhcp"
var (
	DHCPEnabled    = true
	DHCPInterfaces []string
	DHCPLeaseTime  time.Duration
)

// "admin"
var (
	AdminHTTP []string
	AdminSSH  []string
)

var (
	f   *os.File
	err error
)

var (
	customconfig    = false
	configLocations []string
)

var (
	// Debug and Trace are our global debug toggles.
	Debug, Trace bool
	// Filename identifies the location of our configuration file.
	Filename           string
	prefConfigLocation string
	// Snek represents our instance of Viper.
	Snek *viper.Viper
)
