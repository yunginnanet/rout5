package logging

import (
	"io"
	"log/syslog"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"git.tcp.direct/kayos/rout5/config"
)

var (
	// CurrentLogFile is used for accessing the location of the currently used logging file across packages.
	CurrentLogFile string
	logFile        *os.File
	logDir         string
	logger         atomic.Value
	syslogWriter   io.Writer
)

func init() {
	var err error
	syslogWriter, err = syslog.New(syslog.LOG_SYSLOG, "rout5")
	if err != nil {
		syslogWriter = io.Discard
	}
	logger.Store(zerolog.New(zerolog.SyslogLevelWriter(syslogWriter.(*syslog.Writer))))
}

// StartLogger instantiates an instance of our zerolog loggger so we can hook it in our main package.
// While this does return a logger, it should not be used for additional retrievals of the logger. Use GetLogger()
func StartLogger() *zerolog.Logger {
	logDir = config.Snek.GetString("logger.directory")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		println("cannot create logging directory: " + logDir + "(" + err.Error() + ")")
		os.Exit(1)
	}

	tnow := config.Title

	if config.Snek.GetBool("logger.use_date_filename") {
		tnow = strings.ReplaceAll(time.Now().Format(time.RFC822), " ", "_")
		tnow = strings.ReplaceAll(tnow, ":", "-")
	}

	CurrentLogFile = logDir + tnow + ".logging"

	var err error
	if logFile, err = os.OpenFile(CurrentLogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o666); err != nil {
		println("cannot create logging file: " + err.Error())
		os.Exit(1)
	}

	multi := zerolog.MultiLevelWriter(zerolog.ConsoleWriter{NoColor: config.NoColor, Out: os.Stdout}, logFile)
	logger.Store(zerolog.New(multi).With().Timestamp().Logger())
	return GetLogger()
}

var nilLog = zerolog.New(zerolog.New(io.Discard))

// GetLogger retrieves our global logger object
func GetLogger() *zerolog.Logger {
	al := logger.Load()
	if al == nil {
		return &nilLog
	}

	if lg, ok := al.(zerolog.Logger); ok {
		return &lg
	}

	return &nilLog
}
