// Package logger writes the tool's log messages, for people (the console
// transport) or for GitHub Actions to annotate (the github transport).
package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Level is a log level.
type Level int

const (
	Debug Level = 10
	Info  Level = 20
	Warn  Level = 30
	Error Level = 40
	Fatal Level = 50
)

// Levels maps level names to levels.
var Levels = map[string]Level{"debug": Debug, "info": Info, "warn": Warn, "error": Error, "fatal": Fatal}

// LevelNames lists the level names, from the least to the most severe.
var LevelNames = []string{"debug", "info", "warn", "error", "fatal"}

var levelTags = map[Level]string{Debug: "DEBUG", Info: "INFO", Warn: "WARN", Error: "ERROR", Fatal: "FATAL"}

// ANSI colors of each level, as `util.styleText` writes them.
var levelColors = map[Level][2]string{
	Debug: {"\x1b[34m", "\x1b[39m"}, // blue
	Info:  {"\x1b[32m", "\x1b[39m"}, // green
	Warn:  {"\x1b[33m", "\x1b[39m"}, // yellow
	Error: {"\x1b[35m", "\x1b[39m"}, // magenta
	Fatal: {"\x1b[31m", "\x1b[39m"}, // red
}

var githubCommands = map[Level]string{Debug: "debug", Info: "notice", Warn: "warning", Error: "error", Fatal: "error"}

// Position is the span of lines a message is about.
type Position struct {
	Start int
	End   int
}

// File is the file a message is about.
type File struct {
	Path     string
	Position *Position
}

// Fields are a message's metadata: the file it is about, a stack trace, and
// anything else (printed as JSON).
type Fields struct {
	File  *File
	Stack string
	Extra []any // alternating keys and values
}

// Entry is a message handed to a transport.
type Entry struct {
	Level     Level
	Message   string
	Timestamp time.Time
	Fields    Fields
	Module    string
}

// Transport writes entries.
type Transport func(w io.Writer, e Entry)

// Transports are the available transports, by name.
var Transports = map[string]Transport{"console": ConsoleTransport, "github": GitHubTransport}

// Logger logs messages at or above its level. Child loggers start at their
// parent's level, and follow the levels set on it.
type Logger struct {
	mu        *sync.Mutex
	transport Transport
	out       io.Writer
	level     Level
	module    string
	children  []*Logger
}

// New creates a logger writing to stdout with a transport.
func New(transport Transport) *Logger {
	return &Logger{mu: &sync.Mutex{}, transport: transport, out: os.Stdout, level: Info}
}

// Default is the logger the tool logs to.
var Default = New(ConsoleTransport)

// SetOutput changes where the logger and its children write.
func (l *Logger) SetOutput(w io.Writer) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.setOutput(w)
}

func (l *Logger) setOutput(w io.Writer) {
	l.out = w

	for _, c := range l.children {
		c.setOutput(w)
	}
}

// SetTransport changes how the logger and its children write.
func (l *Logger) SetTransport(t Transport) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.setTransport(t)
}

func (l *Logger) setTransport(t Transport) {
	l.transport = t

	for _, c := range l.children {
		c.setTransport(t)
	}
}

// Child creates a logger for a module, at this one's level.
func (l *Logger) Child(module string) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	c := &Logger{mu: l.mu, transport: l.transport, out: l.out, level: l.level, module: module}
	l.children = append(l.children, c)

	return c
}

// SetLevel sets the level of the logger and its children, by name (ignored
// when unknown) or number.
func (l *Logger) SetLevel(level any) {
	var lv Level

	switch v := level.(type) {
	case string:
		known, ok := Levels[strings.ToLower(v)]

		if !ok {
			return
		}

		lv = known
	case Level:
		lv = v
	case int:
		lv = Level(v)
	default:
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.setLevel(lv)
}

func (l *Logger) setLevel(lv Level) {
	l.level = lv

	for _, c := range l.children {
		c.setLevel(lv)
	}
}

// Level returns the current level.
func (l *Logger) Level() Level {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.level
}

func (l *Logger) log(level Level, message any, fields []Fields) {
	if level < l.Level() {
		return
	}

	var f Fields

	if len(fields) > 0 {
		f = fields[0]
	}

	var msg string

	switch m := message.(type) {
	case []string:
		for _, s := range m {
			l.log(level, s, fields)
		}

		return
	case error:
		msg = m.Error()

		if f.Stack == "" {
			f.Stack = fmt.Sprintf("%+v\n", m)

			if f.Stack == msg+"\n" {
				f.Stack = ""
			}
		}
	default:
		msg = fmt.Sprint(m)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.transport(l.out, Entry{Level: level, Message: msg, Timestamp: time.Now(), Fields: f, Module: l.module})
}

func (l *Logger) Debug(message any, fields ...Fields) { l.log(Debug, message, fields) }
func (l *Logger) Info(message any, fields ...Fields)  { l.log(Info, message, fields) }
func (l *Logger) Warn(message any, fields ...Fields)  { l.log(Warn, message, fields) }
func (l *Logger) Error(message any, fields ...Fields) { l.log(Error, message, fields) }
func (l *Logger) Fatal(message any, fields ...Fields) { l.log(Fatal, message, fields) }

// With creates fields from alternating keys and values.
func With(kv ...any) Fields {
	return Fields{Extra: kv}
}

// colorEnabled follows `util.styleText`: colors go to terminals, unless
// NO_COLOR is set, or always when FORCE_COLOR is.
func colorEnabled(w io.Writer) bool {
	if v, ok := os.LookupEnv("FORCE_COLOR"); ok {
		return v != "0" && v != "false"
	}

	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}

	if f, ok := w.(*os.File); ok {
		if info, err := f.Stat(); err == nil {
			return info.Mode()&os.ModeCharDevice != 0
		}
	}

	return false
}

func style(w io.Writer, open, close, text string) string {
	if !colorEnabled(w) {
		return text
	}

	return open + text + close
}

// PrettifyLevel returns a level's (colored) tag.
func PrettifyLevel(w io.Writer, level Level) string {
	tag, ok := levelTags[level]

	if !ok {
		tag = fmt.Sprint(int(level))
	}

	c, ok := levelColors[level]

	if !ok {
		return strings.ToUpper(tag)
	}

	return style(w, c[0], c[1], strings.ToUpper(tag))
}

// PrettifyTimestamp formats a time as `HH:MM:SS.mmm` in UTC.
func PrettifyTimestamp(t time.Time) string {
	return t.UTC().Format("15:04:05.000")
}

// extraJSON writes the extra fields as a JSON object, in order.
func extraJSON(kv []any) string {
	var b strings.Builder

	b.WriteByte('{')

	for i := 0; i+1 < len(kv); i += 2 {
		if i > 0 {
			b.WriteByte(',')
		}

		key, _ := json.Marshal(fmt.Sprint(kv[i]))
		value, err := json.Marshal(kv[i+1])

		if err != nil {
			value, _ = json.Marshal(fmt.Sprint(kv[i+1]))
		}

		b.Write(key)
		b.WriteByte(':')
		b.Write(value)
	}

	b.WriteByte('}')

	return b.String()
}

// ConsoleTransport writes human-friendly lines.
func ConsoleTransport(w io.Writer, e Entry) {
	var b strings.Builder

	b.WriteString("[" + PrettifyTimestamp(e.Timestamp) + "]")
	b.WriteString(" " + PrettifyLevel(w, e.Level))

	if e.Module != "" {
		b.WriteString(" (" + e.Module + ")")
	}

	b.WriteString(": " + e.Message)

	if e.Fields.File != nil {
		b.WriteString(" at " + e.Fields.File.Path)

		if p := e.Fields.File.Position; p != nil {
			fmt.Fprintf(&b, "(%d:%d)", p.Start, p.End)
		}
	}

	if len(e.Fields.Extra) > 0 {
		b.WriteString(" " + style(w, "\x1b[35m", "\x1b[39m", extraJSON(e.Fields.Extra)))
	}

	b.WriteString("\n")
	b.WriteString(e.Fields.Stack)

	io.WriteString(w, b.String())
}

var (
	commandDataEscapes     = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A")
	commandPropertyEscapes = strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C")
)

// GitHubTransport writes GitHub Actions workflow commands, annotating the
// file and lines a message is about.
func GitHubTransport(w io.Writer, e Entry) {
	module := ""

	if e.Module != "" {
		module = " (" + e.Module + ")"
	}

	message := "[" + PrettifyTimestamp(e.Timestamp) + "] " + PrettifyLevel(w, e.Level) + module + ": " + e.Message

	command, ok := githubCommands[e.Level]

	if !ok {
		command = "notice"
	}

	data := commandDataEscapes.Replace(message)

	// Debug messages take no properties
	if command == "debug" {
		io.WriteString(w, "::debug::"+data+"\n")

		return
	}

	var annotations []string

	if f := e.Fields.File; f != nil {
		if f.Path != "" {
			annotations = append(annotations, "file="+commandPropertyEscapes.Replace(f.Path))
		}

		if f.Position != nil && f.Position.Start != 0 {
			annotations = append(annotations, fmt.Sprintf("line=%d", f.Position.Start))
		}

		if f.Position != nil && f.Position.End != 0 {
			annotations = append(annotations, fmt.Sprintf("endLine=%d", f.Position.End))
		}
	}

	io.WriteString(w, "::"+command+" "+strings.Join(annotations, ",")+"::"+data+"\n")
}
