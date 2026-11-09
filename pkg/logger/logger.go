package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

var log zerolog.Logger

func Init(pretty bool) { InitTo(os.Stdout, pretty) }

// InitTo is Init with an explicit output writer.
func InitTo(w io.Writer, pretty bool) {
	zerolog.TimeFieldFormat = time.RFC3339

	if pretty {
		log = zerolog.New(zerolog.ConsoleWriter{Out: w, TimeFormat: time.RFC3339}).
			With().
			Timestamp().
			Logger()
	} else {
		log = zerolog.New(w).
			With().
			Timestamp().
			Logger()
	}

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

func Get() *zerolog.Logger {
	return &log
}
