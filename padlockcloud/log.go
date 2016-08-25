package padlockcloud

import "os"
import "io"
import "log"

type LogConfig struct {
	LogFile string `yaml:"log_file"`
	ErrFile string `yaml:"err_file"`
}

type Log struct {
	Info   *log.Logger
	Error  *log.Logger
	Config *LogConfig
}

func (logger *Log) Init() error {
	var out io.Writer
	var errOut io.Writer
	var err error

	config := logger.Config

	if config.LogFile != "" {
		if out, err = os.OpenFile(config.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err != nil {
			return err
		}
	}

	if config.ErrFile != "" {
		if errOut, err = os.OpenFile(config.ErrFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err != nil {
			return err
		}
	} else {
		errOut = out
	}

	if out == nil {
		out = os.Stdout
	}
	if errOut == nil {
		errOut = os.Stderr
	}

	logger.Info = log.New(out, "INFO: ", log.Ldate|log.Ltime)
	logger.Error = log.New(errOut, "ERROR: ", log.Ldate|log.Ltime)

	return nil
}
