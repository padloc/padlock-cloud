package padlockcloud

import "os"
import "io"
import "log"

type LogConfig struct {
	LogFile      string `yaml:"log_file"`
	ErrFile      string `yaml:"err_file"`
	NotifyErrors string `yaml:"notify_errors"`
}

type Log struct {
	Info   *log.Logger
	Error  *log.Logger
	Sender Sender
	Config *LogConfig
}

type SendWriter struct {
	Sender
	Recipient string
	Subject   string
}

func (sw *SendWriter) Write(p []byte) (int, error) {
	go sw.Send(sw.Recipient, sw.Subject, string(p))
	return len(p), nil
}

func (l *Log) Init() error {
	var out io.Writer
	var errOut io.Writer
	var err error

	config := l.Config

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

	if l.Config.NotifyErrors != "" {
		sw := &SendWriter{
			l.Sender,
			l.Config.NotifyErrors,
			"Padlock Cloud Error Notification",
		}
		errOut = io.MultiWriter(sw, errOut)
	}

	l.Info = log.New(out, "INFO: ", log.Ldate|log.Ltime)
	l.Error = log.New(errOut, "ERROR: ", log.Ldate|log.Ltime)

	return nil
}
