package log

import (
	hclog "github.com/hashicorp/go-hclog"
)

// Logger - Application wide logger obj
var Logger hclog.Logger

// InitLogger - a logger for application wide use
func InitLogger(name, logLevel string) {
	Logger = hclog.New(&hclog.LoggerOptions{
		Name:  name,
		Level: hclog.LevelFromString(logLevel),
	})
}
