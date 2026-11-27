package releaseverify

import (
	"fmt"
	"strings"
)

// shellClass says how a classifier may read one rune of shell text.
type shellClass uint8

const (
	shellPlain   shellClass = iota // unquoted: may separate words and commands
	shellQuoted                    // quoted or escaped: always part of a word
	shellSyntax                    // quote or escape character the shell removes
	shellComment                   // comment text: separates, never quotes
	shellBody                      // heredoc body: separates, never quotes
)

type shellRune struct {
	value rune
	class shellClass
	// nested marks the interior of a $(...), which stays inside one segment.
	nested bool
}

type shellScan struct {
	runes []shellRune
	// substitutions holds the bodies of the outermost command and process
	// substitutions.
	substitutions []string
	backticks     bool
	// err names the line of the first construct the reader cannot close. Every
	// other field is then a best effort that no caller may treat as complete.
	err error
}

type shellFrameKind uint8

const (
	shellTop shellFrameKind = iota
	shellDollarParen
	shellProcessParen
	shellArithmetic
	shellBacktick
)

// shellFrame is one quoting context. A substitution opens a fresh one whatever
// the quote state around it.
type shellFrame struct {
	kind   shellFrameKind
	quote  rune // 0, '\'', '"', or '$' for $'...'
	opened int  // line that opened the frame
	quoted int  // line that opened the quote
	start  int  // index of the first body rune
	parens int
	cases  int
	word   strings.Builder
	// command is true where the next word names a command.
	command bool
}

type shellHeredoc struct {
	delimiter          string
	stripTabs, expands bool
	line               int
}

type shellScanner struct {
	source   []rune
	scan     shellScan
	frames   []*shellFrame
	pending  []shellHeredoc
	position int
	line     int
}

// scanShell is the one quote-state machine behind every shell reader here. It
// classifies text without expanding or executing it.
func scanShell(command string) shellScan {
	scanner := &shellScanner{source: []rune(command), line: 1}
	scanner.frames = []*shellFrame{{kind: shellTop, opened: 1, command: true}}
	for scanner.position < len(scanner.source) {
		scanner.step()
	}
	scanner.finish()
	return scanner.scan
}

// checkShellSyntax reports the first construct the reader cannot follow.
func checkShellSyntax(command string) error { return scanShell(command).err }

func (s *shellScanner) top() *shellFrame { return s.frames[len(s.frames)-1] }

func (s *shellScanner) peek(offset int) rune {
	if s.position+offset >= len(s.source) {
		return 0
	}
	return s.source[s.position+offset]
}

func (s *shellScanner) nested() bool {
	for _, frame := range s.frames {
		if frame.kind == shellDollarParen || frame.kind == shellArithmetic {
			return true
		}
	}
	return false
}

// emit consumes count runes under one class.
func (s *shellScanner) emit(count int, class shellClass) {
	for ; count > 0 && s.position < len(s.source); count-- {
		value := s.source[s.position]
		s.scan.runes = append(s.scan.runes, shellRune{value: value, class: class, nested: s.nested()})
		if value == '\n' {
			s.line++
		}
		s.position++
	}
}

func (s *shellScanner) fail(line int, reason string) {
	if s.scan.err == nil {
		s.scan.err = fmt.Errorf("line %d: %s", line, reason)
	}
}

func (s *shellScanner) open(kind shellFrameKind, width int) {
	opened := s.line
	s.emit(width, shellPlain)
	s.frames = append(s.frames, &shellFrame{kind: kind, opened: opened, start: s.position, command: true})
}

func (s *shellScanner) close(width int) {
	frame := s.top()
	end := s.position
	s.frames = s.frames[:len(s.frames)-1]
	s.emit(width, shellPlain)
	if frame.kind != shellDollarParen && frame.kind != shellProcessParen {
		return
	}
	for _, outer := range s.frames {
		if outer.kind == shellDollarParen || outer.kind == shellProcessParen {
			return
		}
	}
	s.scan.substitutions = append(s.scan.substitutions, string(s.source[frame.start:end]))
}

// expansion opens the substitution or backtick that starts here, if any. These
// are live both unquoted and inside double quotes.
func (s *shellScanner) expansion() bool {
	switch {
	case s.peek(0) == '$' && s.peek(1) == '(' && s.peek(2) == '(':
		s.open(shellArithmetic, 3)
	case s.peek(0) == '$' && s.peek(1) == '(':
		s.open(shellDollarParen, 2)
	case s.peek(0) == '`':
		s.scan.backticks = true
		if s.top().kind == shellBacktick && s.top().quote == 0 {
			s.close(1)
		} else {
			s.open(shellBacktick, 1)
		}
	default:
		return false
	}
	return true
}

func (s *shellScanner) step() {
	frame, character := s.top(), s.peek(0)
	switch frame.quote {
	case '\'':
		if character == '\'' {
			frame.quote = 0
			s.emit(1, shellSyntax)
		} else {
			s.emit(1, shellQuoted)
		}
		return
	case '$':
		switch character {
		case '\\':
			s.emit(1, shellSyntax)
			s.emit(1, shellQuoted)
		case '\'':
			frame.quote = 0
			s.emit(1, shellSyntax)
		default:
			s.emit(1, shellQuoted)
		}
		return
	case '"':
		switch {
		case character == '\\':
			s.escape()
		case character == '"':
			frame.quote = 0
			s.emit(1, shellSyntax)
		case s.expansion():
		default:
			s.emit(1, shellQuoted)
		}
		return
	}
	s.stepUnquoted(frame, character)
}

func (s *shellScanner) escape() {
	if s.position+1 >= len(s.source) {
		s.fail(s.line, "escape at end of input")
	}
	s.emit(1, shellSyntax)
	s.emit(1, shellQuoted)
}

func (s *shellScanner) stepUnquoted(frame *shellFrame, character rune) {
	arithmetic := frame.kind == shellArithmetic
	switch {
	case character == '\\':
		if s.peek(1) != '\n' {
			frame.word.WriteRune(character)
		}
		s.escape()
		return
	case character == '\'' || character == '"':
		frame.quote, frame.quoted = character, s.line
		if character == '\'' && s.position > 0 && s.source[s.position-1] == '$' {
			frame.quote = '$'
		}
		frame.word.WriteRune(character)
		s.emit(1, shellSyntax)
		return
	case character == '#' && !arithmetic && (s.position == 0 || strings.ContainsRune(" \t\r\n;&|()`", s.source[s.position-1])):
		for s.position < len(s.source) && s.peek(0) != '\n' && (frame.kind != shellBacktick || s.peek(0) != '`') {
			s.emit(1, shellComment)
		}
		return
	}
	if s.expansion() {
		frame.word.WriteRune('$')
		return
	}
	if strings.ContainsRune(" \t\r\n;&|()<>", character) {
		s.endWord(frame)
	} else {
		frame.word.WriteRune(character)
	}
	switch {
	case arithmetic && character == ')' && frame.parens == 0:
		if s.peek(1) != ')' {
			s.fail(s.line, "unbalanced arithmetic expansion")
			s.emit(1, shellPlain)
			return
		}
		s.close(2)
		return
	case character == '(' && s.peek(1) == '(' && frame.command && frame.word.Len() == 0:
		s.open(shellArithmetic, 2)
		return
	case character == '(':
		frame.parens++
		frame.command = true
	case character == ')' && frame.parens > 0:
		frame.parens--
		frame.command = true
	case character == ')' && frame.cases > 0:
		// A case pattern's terminator closes nothing.
		frame.command = true
	case character == ')' && (frame.kind == shellDollarParen || frame.kind == shellProcessParen):
		s.close(1)
		return
	case (character == '<' || character == '>') && s.peek(1) == '(' && !arithmetic:
		s.open(shellProcessParen, 2)
		return
	case character == '<' && s.peek(1) == '<' && !arithmetic:
		if s.peek(2) == '<' {
			s.emit(3, shellPlain)
		} else {
			s.heredocOperator()
		}
		return
	case character == '\n':
		frame.command = true
		s.emit(1, shellPlain)
		s.heredocBodies()
		return
	case strings.ContainsRune(";&|", character):
		frame.command = true
	}
	s.emit(1, shellPlain)
}

// endWord tracks case ... esac, whose patterns end in an unmatched ")".
func (s *shellScanner) endWord(frame *shellFrame) {
	word := frame.word.String()
	if word == "" {
		return
	}
	frame.word.Reset()
	if !frame.command {
		return
	}
	switch word {
	case "case":
		frame.cases++
	case "esac":
		frame.cases--
	case "if", "then", "else", "elif", "do", "while", "until", "!", "{":
		return
	}
	frame.command = false
}

// heredocOperator consumes "<<" or "<<-" and queues the delimiter. The
// delimiter text itself is read by the ordinary steps that follow.
func (s *shellScanner) heredocOperator() {
	line := s.line
	s.emit(2, shellPlain)
	heredoc := shellHeredoc{expands: true, line: line}
	if s.peek(0) == '-' {
		heredoc.stripTabs = true
		s.emit(1, shellPlain)
	}
	index := s.position
	for index < len(s.source) && (s.source[index] == ' ' || s.source[index] == '\t') {
		index++
	}
	var delimiter strings.Builder
	var quote rune
	for ; index < len(s.source); index++ {
		character := s.source[index]
		switch {
		case quote != 0 && character == quote:
			quote = 0
		case quote != 0:
			delimiter.WriteRune(character)
		case character == '\'' || character == '"':
			quote, heredoc.expands = character, false
		case character == '\\' && index+1 < len(s.source):
			heredoc.expands = false
			index++
			delimiter.WriteRune(s.source[index])
		case strings.ContainsRune(" \t\r\n;&|()<>`", character):
			index = len(s.source)
		default:
			delimiter.WriteRune(character)
		}
	}
	if delimiter.Len() == 0 {
		s.fail(line, "heredoc without a delimiter")
		return
	}
	heredoc.delimiter = delimiter.String()
	s.pending = append(s.pending, heredoc)
}

// heredocBodies consumes the queued bodies. Their text stays visible to the
// classifiers but never changes the quote state; an unquoted delimiter leaves
// substitutions in the body live.
func (s *shellScanner) heredocBodies() {
	pending := s.pending
	s.pending = nil
	for _, heredoc := range pending {
		closed := false
		for !closed && s.position < len(s.source) {
			end := s.position
			for end < len(s.source) && s.source[end] != '\n' {
				end++
			}
			line := string(s.source[s.position:end])
			if heredoc.stripTabs {
				line = strings.TrimLeft(line, "\t")
			}
			closed = line == heredoc.delimiter
			s.heredocLine(end, heredoc.expands && !closed)
		}
		if !closed {
			s.fail(heredoc.line, "unterminated heredoc "+heredoc.delimiter)
		}
	}
}

// heredocLine consumes through the newline at end, running any substitution to
// its close even when that is on a later line.
func (s *shellScanner) heredocLine(end int, expands bool) {
	depth := len(s.frames)
	for s.position <= end && s.position < len(s.source) {
		switch character := s.peek(0); {
		case expands && character == '\\':
			s.emit(1, shellSyntax)
			if s.position < end {
				s.emit(1, shellBody)
			}
		case expands && (character == '`' || (character == '$' && s.peek(1) == '(')):
			s.expansion()
			for len(s.frames) > depth && s.position < len(s.source) {
				s.step()
			}
			for end < s.position {
				end++
			}
			for end < len(s.source) && s.source[end] != '\n' {
				end++
			}
		case character == '\'' || character == '"':
			s.emit(1, shellSyntax)
		default:
			s.emit(1, shellBody)
		}
	}
}

func (s *shellScanner) finish() {
	frame := s.top()
	switch {
	case frame.quote == '"':
		s.fail(frame.quoted, "unterminated double quote")
	case frame.quote != 0:
		s.fail(frame.quoted, "unterminated single quote")
	case frame.kind == shellBacktick:
		s.fail(frame.opened, "unterminated backtick substitution")
	case frame.kind != shellTop:
		s.fail(frame.opened, "unterminated substitution")
	case len(s.pending) > 0:
		s.fail(s.pending[0].line, "unterminated heredoc "+s.pending[0].delimiter)
	}
}
