// Package logger provides a structured JSON logger built on top of zerolog.
// It exposes a package-level logger that can be used throughout the application
// and a Gin-compatible middleware for HTTP request logging.
package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Logger is the package-level zerolog logger. It is initialized by calling
// Init and can be used directly via the zerolog global log package or through
// the exported Logger variable.
var Logger zerolog.Logger

// Level represents a logging level.
type Level = zerolog.Level

// Log level constants for use with Options.Level.
const (
	DebugLevel = zerolog.DebugLevel
	InfoLevel  = zerolog.InfoLevel
	WarnLevel  = zerolog.WarnLevel
	ErrorLevel = zerolog.ErrorLevel
)

// Options configures the logger.
type Options struct {
	// Level sets the minimum log level. Defaults to InfoLevel.
	Level Level
	// Pretty enables human-readable console output instead of JSON.
	// Should only be used in development.
	Pretty bool
	// Output is the writer to send log output to. Defaults to os.Stdout.
	Output io.Writer
}

// Init initializes the package-level logger with the given options and sets
// the zerolog global logger so that log.Info(), log.Error(), etc. all use the
// same configuration.
func Init(opts Options) {
	if opts.Output == nil {
		opts.Output = os.Stdout
	}

	writer := opts.Output
	if opts.Pretty {
		writer = zerolog.ConsoleWriter{
			Out:        opts.Output,
			TimeFormat: time.RFC3339,
		}
	}

	zerolog.TimeFieldFormat = time.RFC3339

	Logger = zerolog.New(writer).
		Level(opts.Level).
		With().
		Timestamp().
		Logger()

	// Override the global zerolog logger so third-party packages that use
	// log.Info() etc. also emit structured JSON.
	log.Logger = Logger
}

// With returns a new logger with the given key-value pairs added as fields.
// This is a convenience wrapper around zerolog's With().Str() chain.
func With(key, value string) zerolog.Logger {
	return Logger.With().Str(key, value).Logger()
}

// Debug logs a message at debug level.
func Debug(msg string) {
	Logger.Debug().Msg(msg)
}

// Info logs a message at info level.
func Info(msg string) {
	Logger.Info().Msg(msg)
}

// Warn logs a message at warn level.
func Warn(msg string) {
	Logger.Warn().Msg(msg)
}

// Error logs a message at error level with an optional error.
func Error(msg string, err error) {
	Logger.Error().Err(err).Msg(msg)
}

// Fatal logs a message at fatal level and then calls os.Exit(1).
func Fatal(msg string, err error) {
	Logger.Fatal().Err(err).Msg(msg)
}
