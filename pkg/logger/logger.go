package logger

import (
	"os"
	"time"

	"github.com/rs/zerolog"
)

var log zerolog.Logger

func Init(pretty bool) {
	zerolog.TimeFieldFormat = time.RFC3339

	if pretty {
		log = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}).
			With().
			Timestamp().
			Logger()
	} else {
		log = zerolog.New(os.Stdout).
			With().
			Timestamp().
			Logger()
	}

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

func Get() *zerolog.Logger {
	return &log
}
