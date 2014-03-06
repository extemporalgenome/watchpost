package main

import "fmt"
import "log"

type LogLevel int

var LogLvl LogLevel

const (
	LvlDebug LogLevel = iota
	LvlInfo
	LvlError
)

var levels = []string{"[DEBUG]", "[INFO] ", "[ERROR]"}

func (lvl LogLevel) String() string {
	return levels[lvl]
}

func Log(lvl LogLevel, format string, args ...interface{}) {
	if LogLvl <= lvl {
		log.Println(lvl, fmt.Sprintf(format, args...))
	}
}
