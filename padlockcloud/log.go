package padlockcloud

import "os"
import "io"
import "log"

var stdout io.Writer = os.Stdout
var stderr io.Writer = os.Stderr

type LogConfig struct {
	// File to write logs to
	LogFile string `yaml:"log_file"`
	// File to write errors to. Defaults to the value of `LogFile`
	ErrFile string `yaml:"err_file"`
	// An address to send error notifications to
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
		out = stdout
	}
	if errOut == nil {
		errOut = stderr
	}

	if l.Config.NotifyErrors != "" && l.Sender != nil {
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

func (l *Log) InitWithConfig(config *LogConfig) {
	l.Config = config
	l.Init()
}

func NewLog(config *LogConfig, sender Sender) *Log {
	l := &Log{Sender: sender}
	l.InitWithConfig(config)
	return l
}
