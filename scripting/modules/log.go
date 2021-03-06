package modules

import (
	"github.com/bbuck/dragon-mud/logger"
	"github.com/bbuck/dragon-mud/scripting/keys"
	"github.com/bbuck/dragon-mud/scripting/lua"
)

// Log is the definition of the Lua logging module.
//   error(msg[, data])
//     @param msg: string = the message to log according the log configuraiton
//       provided for the application
//     @param data: table = associated data to log with the message, if any
//       additional data is required.
//     log message with data on the error level, data can be omitted or nil
//   warn(msg[, data])
//     @param msg: string = the message to log according the log configuraiton
//       provided for the application
//     @param data: table = associated data to log with the message, if any
//       additional data is required.
//     log message with data on the warn level, data can be omitted or nil
//   info(msg[, data])
//     @param msg: string = the message to log according the log configuraiton
//       provided for the application
//     @param data: table = associated data to log with the message, if any
//       additional data is required.
//     log message with data on the info level, data can be omitted or nil
//   debug(msg[, data])
//     @param msg: string = the message to log according the log configuraiton
//       provided for the application
//     @param data: table = associated data to log with the message, if any
//       additional data is required.
//     log message with data on the debug level, data can be omitted or nil
var Log = lua.TableMap{
	"error": func(eng *lua.Engine) int {
		performLog(eng, func(l logger.Log, msg string) {
			l.Error(msg)
		})

		return 0
	},
	"warn": func(eng *lua.Engine) int {
		performLog(eng, func(l logger.Log, msg string) {
			l.Warn(msg)
		})

		return 0
	},
	"info": func(eng *lua.Engine) int {
		performLog(eng, func(l logger.Log, msg string) {
			l.Info(msg)
		})

		return 0
	},
	"debug": func(eng *lua.Engine) int {
		performLog(eng, func(l logger.Log, msg string) {
			l.Debug(msg)
		})

		return 0
	},
}

func loggerForEngine(eng *lua.Engine) logger.Log {
	if log, ok := eng.Meta[keys.Logger].(logger.Log); ok {
		return log
	}

	name := "engine(unknown)"
	if n, ok := eng.Meta[keys.EngineID].(string); ok {
		name = n
	}

	l := logger.NewWithSource(name)
	eng.Meta[keys.Logger] = l

	return l
}

func performLog(eng *lua.Engine, fn func(logger.Log, string)) {
	data := eng.Nil()
	if eng.StackSize() >= 2 {
		data = eng.PopTable()
	}
	msg := eng.PopString()

	log := loggerForEngine(eng)

	if !data.IsNil() && data.IsTable() {
		m := data.AsMapStringInterface()
		log = log.WithFields(logger.Fields(m))
	}

	fn(log, msg)
}
