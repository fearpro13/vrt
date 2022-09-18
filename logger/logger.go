package logger

import (
	"fmt"
	"time"
)

const JunkLogName = "JUNK"
const JunkLogLvl = 0

const DebugLogName = "DEBUG"
const DebugLogLvl = 1

const InfoLogName = "INFO"
const InfoLogLvl = 2

const NoticeLogName = "NOTICE"
const NoticeLogLvl = 3

const WarningLogName = "WARN"
const WarningLogLvl = 4

const ErrorLogName = "ERR"
const ErrorLogLvl = 5

const CriticalLogName = "CRITICAL"
const CriticalLogLvl = 6

const EmergencyLogName = "EMERGENCY"
const EmergencyLogLvl = 7

var globalLogLvl = 0

func SetLogLevel(logLevel int) {
	globalLogLvl = logLevel
}

func Junk(message string) {
	if globalLogLvl > JunkLogLvl {
		return
	}
	displayMessage(template(message, JunkLogName))
}

func Debug(message string) {
	if globalLogLvl > DebugLogLvl {
		return
	}
	displayMessage(template(message, DebugLogName))
}

func Info(message string) {
	if globalLogLvl > InfoLogLvl {
		return
	}
	displayMessage(template(message, InfoLogName))
}

func Notice(message string) {
	if globalLogLvl > NoticeLogLvl {
		return
	}
	displayMessage(template(message, NoticeLogName))
}

func Warning(message string) {
	if globalLogLvl > WarningLogLvl {
		return
	}
	displayMessage(template(message, WarningLogName))
}

func Error(message string) {
	if globalLogLvl > ErrorLogLvl {
		return
	}
	displayMessage(template(message, ErrorLogName))
}

func Critical(message string) {
	if globalLogLvl > CriticalLogLvl {
		return
	}
	displayMessage(template(message, CriticalLogName))
}

func Emergency(message string) {
	if globalLogLvl > EmergencyLogLvl {
		return
	}
	displayMessage(template(message, EmergencyLogName))
}

func template(message string, logLevel string) string {
	now := time.Now()
	nowString := now.Format("2006-Jan-2 15:04:05")
	return fmt.Sprintf("[%s][%s]: %s", nowString, logLevel, message)
}

func displayMessage(message string) {
	fmt.Println(message)
}
