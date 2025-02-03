package logger

import (
	"fmt"
	"log"
	"os"
	"time"
)

type Logger struct {
	*log.Logger
}

func New() *Logger {
	prefix := "\033[36mduh\033[0m \033[90m>\033[0m "
	logger := log.New(os.Stdout, prefix, 0)
	return &Logger{logger}
}

func (l *Logger) Info(format string, v ...interface{}) {
	l.log("\033[90m[%s]\033[0m %s", time.Now().Format("2006/01/02 15:04:05.000000"), fmt.Sprintf(format, v...))
}

func (l *Logger) Warn(format string, v ...interface{}) {
	l.log("\033[33m%s\033[0m", fmt.Sprintf(format, v...))
}

func (l *Logger) Fatal(format string, v ...interface{}) {
	l.Logger.Fatalf(format, v...)
}

func (l *Logger) Link(url string) string {
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", url, url)
}

func (l *Logger) log(format string, v ...interface{}) {
	_ = l.Output(2, fmt.Sprintf(format, v...))
}
