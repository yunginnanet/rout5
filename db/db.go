package db

import (
	"net"
	"sync"
	"time"

	"git.tcp.direct/kayos/common"
	"git.tcp.direct/kayos/database"
	"git.tcp.direct/kayos/database/bitcask"
	"github.com/rs/zerolog/log"

	"git.tcp.direct/kayos/rout5/config"
)

var defaultLease = time.Duration(24) * time.Hour

type key uint8
type subkey uint8

const (
	KeyDHCP key = iota
	KeyRoutes
	KeyFilter
	KeyNAT
	KeyConfig
)

const (
	DHCPv4 subkey = iota
	DHCPv6
	Wire
	FilterIncoming
	FilterOutgoing
	NATPrerouting
	NATPostrouting
	DefaultLeaseTime
)

var (
	dbtarget  = []byte{byte(KeyConfig), byte(DefaultLeaseTime)}
	dhcp4wire = []byte{byte(KeyDHCP), byte(DHCPv4), byte(Wire)}
)

type Lease interface {
	HardwareAddr() string
	Num() int
	Addr() net.IP
	Hostname() string
	HostnameOverride() string
	Expiry() time.Time
}

var (
	// data is our main database object.
	data      *bitcask.DB
	dataSetup = &sync.Once{}
	stores    = []string{"config", "dhcp4", "dhcp6", "nat", "fwin", "fwout"}
)

func db() *bitcask.DB {
	dataSetup.Do(func() {
		data = bitcask.OpenDB(config.DataDirectory)
		for _, s := range stores {
			if iErr := data.Init(s); iErr != nil {
				log.Fatal().Caller().Err(iErr).Msg("db initialization failed")
			}
		}
	})
	return data
}

func DHCPMessages() ([]byte, error) {
	return db().With("dhcp4").Get(dhcp4wire)
}

func cfg() database.Filer {
	return db().With("cfg")
}

func defaultLeaseTime() time.Duration {
	if !cfg().Has(dbtarget) {
		return defaultLease
	}
	ltb, err := cfg().Get(dbtarget)
	if err != nil {
		log.Warn().Caller().Err(err).Msg("failed to get configured default lease despite it's existence")
		return defaultLease
	}
	return time.Duration(common.BytesToFloat64(ltb))
}

func putDHCP4(m net.HardwareAddr, a net.IP) error {
	return db().With("dhcp4").Bitcask.PutWithTTL(m, a.To4(), defaultLeaseTime())
}

func NewDHCP4Lease(m net.HardwareAddr, a net.IP) error {
	return putDHCP4(m, a)
}
