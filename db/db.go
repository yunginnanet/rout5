package db

import (
	"errors"
	"net"
	"time"

	"git.tcp.direct/kayos/database/bitcask"
)

var stores = []string{
	"dhcp4,dhcp6,routes,filters,config",
}

var ErrNotInitialized = errors.New("database is not initialized")

// Data is our main database object.
var Data *bitcask.DB

func InitializeDatabase(path string) *bitcask.DB {
	if Data != nil {
		return Data
	}
	Data = bitcask.OpenDB(path)
	for _, s := range stores {
		Data.Init(s)
	}
	return Data
}

func getLeaseTime() time.Duration {

}

func putDHCP4(m net.HardwareAddr, a net.IP) {
	Data.With("dhcp4").Bitcask.PutWithTTL([]byte(m), a.To16(), getLeaseTime())
}

func NewDHCP4Lease(m net.HardwareAddr, a net.IP) error {
	if Data == nil {
		return ErrNotInitialized
	}

}
