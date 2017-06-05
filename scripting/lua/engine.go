// Copyright (c) 2016-2017 Brandon Buck

package lua

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"unicode"

	"github.com/yuin/gopher-lua"
	luar "layeh.com/gopher-luar"
)

// Engine struct stores a pointer to a gluaLState providing a simplified API.
type Engine struct {
	state *lua.LState
	Meta  map[string]interface{}
}

// ScriptFunction is a type alias for a function that receives an Engine and
// returns an int.
type ScriptFunction func(*Engine) int

// TableMap interface to speed along the creation of table defining maps
// when creating Go modueles for use in Lua.
type TableMap map[string]interface{}

// NewEngine creates a new engine containing a new lua.LState.
func NewEngine(opts ...EngineOptions) *Engine {
	eng := &Engine{
		state: lua.NewState(lua.Options{
			SkipOpenLibs:        true,
			IncludeGoStackTrace: true,
		}),
		Meta: make(map[string]interface{}),
	}
	eng.OpenBase()
	eng.OpenPackage()
	eng.OpenTable()
	eng.OpenString()

	eng.configureFromOptions(opts)

	return eng
}

// perform configuartion work on the engine
func (e *Engine) configureFromOptions(options []EngineOptions) {
	openedLibs := false

	for _, opt := range options {
		if !openedLibs && opt.OpenLibs {
			e.OpenLibs()
		}

		config := luar.GetConfig(e.state)
		switch opt.FieldNaming {
		case SnakeCaseExportedNames:
			config.FieldNames = nil
		case SnakeCaseNames:
			config.FieldNames = func(t reflect.Type, s reflect.StructField) []string {
				return []string{toSnake(s.Name)}
			}
		case ExportedNames:
			config.FieldNames = func(t reflect.Type, s reflect.StructField) []string {
				return []string{s.Name}
			}
		}

		switch opt.MethodNaming {
		case SnakeCaseExportedNames:
			config.MethodNames = nil
		case SnakeCaseNames:
			config.MethodNames = func(t reflect.Type, s reflect.Method) []string {
				return []string{toSnake(s.Name)}
			}
		case ExportedNames:
			config.MethodNames = func(t reflect.Type, s reflect.Method) []string {
				return []string{s.Name}
			}
		}
	}
}

// Close will perform a close on the Lua state.
func (e *Engine) Close() {
	e.state.Close()
}

// OpenBase allows the Lua engine to open the base library up for use in
// scripts.
func (e *Engine) OpenBase() int {
	return lua.OpenBase(e.state)
}

// OpenChannel allows the Lua module for Go channel support to be accessible
// to scripts.
func (e *Engine) OpenChannel() int {
	return lua.OpenChannel(e.state)
}

// OpenCoroutine allows the Lua module for goroutine suppor tto be accessible
// to scripts.
func (e *Engine) OpenCoroutine() int {
	return lua.OpenCoroutine(e.state)
}

// OpenDebug allows the Lua module support debug features to be accissible
// in scripts.
func (e *Engine) OpenDebug() int {
	return lua.OpenDebug(e.state)
}

// OpenIO allows the input/output Lua module to be accessbile in scripts.
func (e *Engine) OpenIO() int {
	return lua.OpenIo(e.state)
}

// OpenMath allows the Lua math moduled to be accessible in scripts.
func (e *Engine) OpenMath() int {
	return lua.OpenMath(e.state)
}

// OpenOS allows the OS Lua module to be accessible in scripts.
func (e *Engine) OpenOS() int {
	return lua.OpenOs(e.state)
}

// OpenPackage allows the Lua module for packages to be used in scripts.
// TODO: Find out what this does/means.
func (e *Engine) OpenPackage() int {
	return lua.OpenPackage(e.state)
}

// OpenString allows the Lua module for string operations to be used in
// scripts.
func (e *Engine) OpenString() int {
	return lua.OpenString(e.state)
}

// OpenTable allows the Lua module for table operations to be used in scripts.
func (e *Engine) OpenTable() int {
	return lua.OpenTable(e.state)
}

// OpenLibs seeds the engine with some basic library access. This should only
// be used if security isn't necessarily a major concern.
func (e *Engine) OpenLibs() {
	e.state.OpenLibs()
}

// DoFile runs the file through the Lua interpreter.
func (e *Engine) DoFile(fn string) error {
	return e.state.DoFile(fn)
}

// LoadString runs the given string through the Lua interpreter, wrapping it
// in a function that is then returned and it can be executed by calling the
// returned function.
func (e *Engine) LoadString(src string) (*Value, error) {
	fn, err := e.state.LoadString(src)
	if err != nil {
		return nil, err
	}

	return e.ValueFor(fn), nil
}

// LoadFile attempts to read the file from the file system and then load it
// into the engine, returning a function that executes the contents of the file.
func (e *Engine) LoadFile(fpath string) (*Value, error) {
	fn, err := e.state.LoadFile(fpath)
	if err != nil {
		return nil, err
	}

	return e.ValueFor(fn), nil
}

// DoString runs the given string through the Lua interpreter.
func (e *Engine) DoString(src string) error {
	return e.state.DoString(src)
}

// RaiseError will throw an error in the Lua engine.
func (e *Engine) RaiseError(err string, args ...interface{}) {
	e.state.RaiseError(err, args...)
}

// ArgumentError raises an error associated with an invalid argument.
func (e *Engine) ArgumentError(n int, msg string) {
	e.state.ArgError(n, msg)
}

// SetGlobal allows for setting global variables in the loaded code.
func (e *Engine) SetGlobal(name string, val interface{}) {
	v := e.ValueFor(val)

	e.state.SetGlobal(name, v.lval)
}

// GetGlobal returns the value associated with the given name, or LuaNil
func (e *Engine) GetGlobal(name string) *Value {
	lv := e.state.GetGlobal(name)

	return e.newValue(lv)
}

// SetField applies the value to the given table associated with the given
// key.
func (e *Engine) SetField(tbl *Value, key string, val interface{}) {
	v := e.ValueFor(val)
	e.state.SetField(tbl.lval, key, v.lval)
}

// RegisterFunc registers a Go function with the script. Using this method makes
// Go functions accessible through Lua scripts.
func (e *Engine) RegisterFunc(name string, fn interface{}) {
	var lfn lua.LValue
	if sf, ok := fn.(func(*Engine) int); ok {
		lfn = e.genScriptFunc(sf)
	} else {
		v := e.ValueFor(fn)
		lfn = v.lval
	}
	e.state.SetGlobal(name, lfn)
}

// RegisterModule takes the values given, maps them to a LuaTable and then
// preloads the module with the given name to be consumed in Lua code.
func (e *Engine) RegisterModule(name string, fields map[string]interface{}) *Value {
	table := e.NewTable()
	for key, val := range fields {
		if sf, ok := val.(func(*Engine) int); ok {
			table.RawSet(key, e.genScriptFunc(sf))
		} else {
			table.RawSet(key, e.ValueFor(val).lval)
		}
	}

	loader := func(l *lua.LState) int {
		l.Push(table.lval)

		return 1
	}
	e.state.PreloadModule(name, loader)

	return table
}

// GetEnviron returns the Environment core table from Lua.
func (e *Engine) GetEnviron() *Value {
	return e.Get(lua.EnvironIndex)
}

// GetRegistry retursn the Registry core table from Lua.
func (e *Engine) GetRegistry() *Value {
	return e.Get(lua.RegistryIndex)
}

// GetGlobals returns the global core table from Lua.
func (e *Engine) GetGlobals() *Value {
	return e.Get(lua.GlobalsIndex)
}

// Get returns the value at the specified location on the Lua stack.
func (e *Engine) Get(n int) *Value {
	lv := e.state.Get(n)
	return e.newValue(lv)
}

// PopValue returns the top value on the Lua stack.
// This method is used to get arguments given to a Go function from a Lua script.
// This method will return a Value pointer that can then be converted into
// an appropriate type.
func (e *Engine) PopValue() *Value {
	val := e.Get(-1)
	e.state.Pop(1)
	if val.IsTable() {
		val.owner = e
	}

	return val
}

// PushValue pushes the given Value onto the Lua stack.
// Use this method when 'returning' values from a Go function called from a
// Lua script.
func (e *Engine) PushValue(val interface{}) {
	v := e.ValueFor(val)
	e.state.Push(v.lval)
}

// StackSize returns the maximum value currently remaining on the stack.
func (e *Engine) StackSize() int {
	return e.state.GetTop()
}

// PopBool returns the top of the stack as an actual Go bool.
func (e *Engine) PopBool() bool {
	v := e.PopValue()

	return v.AsBool()
}

// PopFunction is an alias for PopArg, provided for readability when specifying
// the desired value from the top of the stack.
func (e *Engine) PopFunction() *Value {
	return e.PopValue()
}

// PopInt returns the top of the stack as an actual Go int.
func (e *Engine) PopInt() int {
	v := e.PopValue()
	i := int(v.AsNumber())

	return i
}

// PopInt64 returns the top of the stack as an actual Go int64.
func (e *Engine) PopInt64() int64 {
	v := e.PopValue()
	i := int64(v.AsNumber())

	return i
}

// PopFloat returns the top of the stack as an actual Go float.
func (e *Engine) PopFloat() float64 {
	v := e.PopValue()

	return v.AsFloat()
}

// PopNumber is an alias for PopArg, provided for readability when specifying
// the desired value from the top of the stack.
func (e *Engine) PopNumber() *Value {
	return e.PopValue()
}

// PopString returns the top of the stack as an actual Go string value.
func (e *Engine) PopString() string {
	v := e.PopValue()

	return v.AsString()
}

// PopTable is an alias for PopArg, provided for readability when specifying
// the desired value from the top of the stack.
func (e *Engine) PopTable() *Value {
	tbl := e.PopValue()
	tbl.owner = e

	return tbl
}

// PopInterface returns the top of the stack as an actual Go interface.
func (e *Engine) PopInterface() interface{} {
	v := e.PopValue()

	return v.Interface()
}

// True returns a value for the constant 'true' in Lua.
func (e *Engine) True() *Value {
	return e.newValue(lua.LTrue)
}

// False returns a value for the constant 'false' in Lua.
func (e *Engine) False() *Value {
	return e.newValue(lua.LFalse)
}

// Nil returns a value for the constant 'nil' in Lua.
func (e *Engine) Nil() *Value {
	return e.newValue(lua.LNil)
}

// SecureRequire will set a require function that limits the files that can be
// loaded into the engine.
func (e *Engine) SecureRequire(validPaths []string) {
	require := func(eng *Engine) int {
		if eng.StackSize() == 0 {
			eng.ArgumentError(1, "expected a string, got nothing")
		}
		mod := eng.PopString()
		mod = strings.Replace(mod, ".", "/", -1)
		for _, path := range validPaths {
			fpath := strings.Replace(path, "?", mod, -1)
			if _, err := os.Stat(fpath); err == nil {
				fn, err := eng.LoadFile(fpath)
				if err != nil {
					eng.RaiseError(err.Error())

					return 0
				}
				eng.PushValue(fn)

				return 1
			}
		}

		eng.RaiseError("%q module not found", mod)

		return 0
	}

	tbl := e.NewTable()
	tbl.RawSetInt(1, preloadLoader)
	tbl.RawSetInt(2, require)
	e.GetEnviron().RawGet("package").RawSet("loaders", tbl)
	e.GetRegistry().RawSet("_LOADERS", tbl)
}

// Call allows for calling a method by name.
// The second parameter is the number of return values the function being
// called should return. These values will be returned in a slice of Value
// pointers.
func (e *Engine) Call(name string, retCount int, params ...interface{}) ([]*Value, error) {
	luaParams := make([]lua.LValue, len(params))
	for i, iface := range params {
		v := e.ValueFor(iface)
		luaParams[i] = v.lval
	}

	err := e.state.CallByParam(lua.P{
		Fn:      e.state.GetGlobal(name),
		NRet:    retCount,
		Protect: true,
	}, luaParams...)

	if err != nil {
		return nil, err
	}

	retVals := make([]*Value, retCount)
	for i := retCount - 1; i >= 0; i-- {
		retVals[i] = e.ValueFor(e.state.Get(-1))
		e.state.Pop(1)
	}

	return retVals, nil
}

// RegisterType creates a construtor with the given name that will generate the
// given type.
func (e *Engine) RegisterType(name string, val interface{}) {
	cons := luar.NewType(e.state, val)
	e.state.SetGlobal(name, cons)
}

// RegisterClass assigns a new type, but instead of creating it via "TypeName()"
// it provides a more OO way of creating the object "TypeName.new()" otherwise
// it's functionally equivalent to RegisterType.
func (e *Engine) RegisterClass(name string, val interface{}) {
	cons := luar.NewType(e.state, val)
	table := e.NewTable()
	table.RawSet("new", cons)
	e.state.SetGlobal(name, table.lval)
}

// RegisterClassWithCtor does the same thing as RegisterClass excep the new
// function is mapped to the constructor passed in.
func (e *Engine) RegisterClassWithCtor(name string, typ interface{}, cons interface{}) {
	luar.NewType(e.state, typ)
	lcons := e.ValueFor(cons)
	table := e.NewTable()
	table.RawSet("new", lcons)

	e.state.SetGlobal(name, table.lval)
}

// ValueFor takes a Go type and creates a lua equivalent Value for it.
func (e *Engine) ValueFor(val interface{}) *Value {
	switch v := val.(type) {
	case ScriptableObject:
		return e.newValue(luar.New(e.state, v.ScriptObject()))
	case *Value:
		return v
	case ScriptFunction:
		return e.newValue(luar.New(e.state, e.genScriptFunc(v)))
	case func(*Engine) int:
		return e.newValue(luar.New(e.state, e.genScriptFunc(ScriptFunction(v))))
	default:
		return e.newValue(luar.New(e.state, val))
	}
}

// TableFromMap takes a map of go values and generates a Lua table representing
// the value.
func (e *Engine) TableFromMap(i interface{}) *Value {
	t := e.NewTable()
	m := reflect.ValueOf(i)
	if m.Kind() == reflect.Map {
		for _, k := range m.MapKeys() {
			v := m.MapIndex(k)
			switch v.Kind() {
			case reflect.Map:
				t.Set(k.Interface(), e.TableFromMap(v.Interface()))
			case reflect.Slice:
				t.Set(k.Interface(), e.TableFromSlice(v.Interface()))
			default:
				t.Set(k.Interface(), v.Interface())
			}
		}
	}

	return t
}

// TableFromSlice converts the given slice into a table ready for use in Lua.
func (e *Engine) TableFromSlice(i interface{}) *Value {
	t := e.NewTable()
	s := reflect.ValueOf(i)
	if s.Kind() == reflect.Slice {
		for i := 0; i < s.Len(); i++ {
			v := s.Index(i)
			switch v.Kind() {
			case reflect.Map:
				t.Append(e.TableFromMap(v.Interface()))
			case reflect.Slice:
				t.Append(e.TableFromSlice(v.Interface()))
			default:
				t.Append(s.Index(i).Interface())
			}
		}
	}

	return t
}

// newValue constructs a new value from an LValue.
func (e *Engine) newValue(val lua.LValue) *Value {
	return &Value{
		lval:  val,
		owner: e,
	}
}

// NewTable create and returns a new NewTable.
func (e *Engine) NewTable() *Value {
	tbl := e.newValue(e.state.NewTable())
	tbl.owner = e

	return tbl
}

// NewUserData creates a Lua User Data object from teh given value and
// metatable value.
func (e *Engine) NewUserData(val interface{}, mt interface{}) *Value {
	ud := e.state.NewUserData()
	ud.Value = val
	mtVal := e.ValueFor(mt)
	if mtVal.IsTable() {
		ud.Metatable = mtVal.asTable()
	}

	return e.newValue(ud)
}

// wrapScriptFunction turns a ScriptFunction into a lua.LGFunction
func (e *Engine) wrapScriptFunction(fn ScriptFunction) lua.LGFunction {
	return func(l *lua.LState) int {
		return fn(e)
	}
}

// genScriptFunc will wrap a ScriptFunction with a function that gopher-lua
// expects to see when calling method from Lua.
func (e *Engine) genScriptFunc(fn ScriptFunction) *lua.LFunction {
	return e.state.NewFunction(e.wrapScriptFunction(fn))
}

// ToSnake convert the given string to snake case following the Golang format:
// acronyms are converted to lower-case and preceded by an underscore.
// found at: https://gist.github.com/elwinar/14e1e897fdbe4d3432e1
func toSnake(in string) string {
	runes := []rune(in)
	length := len(runes)

	var out []rune
	for i := 0; i < length; i++ {
		if i > 0 && unicode.IsUpper(runes[i]) && ((i+1 < length && unicode.IsLower(runes[i+1])) || unicode.IsLower(runes[i-1])) {
			out = append(out, '_')
		}
		out = append(out, unicode.ToLower(runes[i]))
	}

	return string(out)
}

// preload loader, pulled from yuin/gopher-lua and converted to match the new
// engine API.
func preloadLoader(eng *Engine) int {
	if eng.StackSize() == 0 {
		eng.ArgumentError(1, "expected a string, but got nothing.")

		return 0
	}

	name := eng.PopString()
	preload := eng.GetEnviron().RawGet("package").RawGet("preload")
	if !preload.IsTable() {
		eng.RaiseError("package.preload must be a table")
	}
	mod := preload.RawGet(name)
	if mod.IsNil() {
		eng.PushValue(fmt.Sprintf("no field package.preload['%s']", name))

		return 1
	}

	eng.PushValue(mod)

	return 1
}
