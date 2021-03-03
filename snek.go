// The snek package implements lightweight subcommands for a command-line
// application.
//
// An example of a main entry point:
//
//     var Program = snek.NewProgram("", os.Args)
//
//     func main() {
//     	Program.Main()
//     }
//
// An example of a subcommand:
//
//     func init() {
//     	Program.Register(snek.Def{
//     		Name:        "echo",
//     		Summary:     "Display text.",
//     		Arguments:   "[-n] [TEXT...]",
//     		Description: `Write the given arguments to standard output.`,
//     		New:         func() snek.Command { return &EchoCommand{} },
//     	})
//     }
//
//     type EchoCommand struct {
//     	NoNewline bool
//     }
//
//     func (c *EchoCommand) SetFlags(flags snek.FlagSet) {
//     	flags.BoolVar(&c.NoNewline, "n", false, "Suppress trailing newline.")
//     }
//
//     func (c *EchoCommand) Run(opt snek.Options) error {
//     	if err := opt.ParseFlags(); err != nil {
//     		return err
//     	}
//     	out := strings.Join(opt.Args(), " ")
//     	fmt.Print(out)
//     	if !c.NoNewline {
//     		fmt.Print("\n")
//     	}
//     	return nil
//     }
//
package snek

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"
)

// Program represents a command-line program.
type Program struct {
	Input
	registry
}

// NewProgram returns a Program initialized with the given raw arguments (e.g.
// os.Args).
//
// name is the name of the program. If empty, then the first argument is used
// instead.
//
// The program is initialized with a default "help" subcommand. If necessary,
// this command can be removed with the NoHelp method.
func NewProgram(name string, args []string) *Program {
	program := Program{
		registry: registry{"help": helpDef},
	}
	program.Program = name
	if len(args) > 0 {
		if program.Program == "" {
			program.Program = args[0]
		}
		program.Arguments = args[1:]
	}
	program.Stdin = os.Stdin
	program.Stdout = os.Stdout
	program.Stderr = os.Stderr
	return &program
}

// Usage sets the GlobalUsage field.
func (p *Program) Usage(usage string) *Program {
	p.GlobalUsage = usage
	return p
}

// writeUsage writes the GlobalUsage message to w, or Stderr if w is nil.
func writeUsage(w io.Writer, i Input, r registry) {
	if w == nil {
		if w = i.Stderr; w == nil {
			return
		}
	}
	var summaries strings.Builder
	r.WriteSummary(&summaries)
	globalUsage := i.GlobalUsage
	if globalUsage == "" {
		globalUsage = "Usage: %s <command>\n\nThe following commands are available:\n%s"
	}
	fmt.Fprintf(i.Stderr, globalUsage, i.Program, summaries.String())
}

// WriteUsage prints the GlobalUsage message to w, or Stderr if w is nil.
func (p *Program) WriteUsage(w io.Writer) {
	writeUsage(w, p.Input, p.registry)
}

// NoHelp unregisters the "help" subcommand.
func (p *Program) NoHelp() *Program {
	delete(p.registry, "help")
	return p
}

// Register registers a subcommand under def.Name. Panics if def.Name is empty,
// if def.New is nil, or if a subcommand was already registered with the name.
func (p *Program) Register(def Def) {
	if def.Name == "" {
		panic("empty New field")
	}
	if def.New == nil {
		panic("empty New field")
	}
	if _, ok := p.registry[def.Name]; ok {
		panic("already registered " + def.Name)
	}
	p.registry[def.Name] = def
}

// Prepare prepares a subcommand. Expects the first argument of p to be the name
// of a subcommand to run. Returns the name and an input to be passed to the
// subcommand.
//
// If there are not enough arguments, or no subcommand of the specified name is
// registered, then an empty string is returned.
func (p *Program) Prepare() (name string, input Input) {
	if len(p.Arguments) < 1 {
		return "", input
	}
	name = p.Arguments[0]
	if !p.Has(name) {
		return "", input
	}
	input = p.Input
	input.Arguments = input.Arguments[1:]
	return name, input
}

// RunWithInput executes the subcommand mapped to the given name with the given
// input. Returns an UnknownCommand error if the name is not a registered
// subcommand.
func (p *Program) RunWithInput(name string, input Input) error {
	def := p.registry[name]
	if def.New == nil {
		return UnknownCommand{Name: name}
	}
	cmd := def.New()
	opt := Options{
		FlagSet:  flag.NewFlagSet(p.Program, flag.ContinueOnError),
		Input:    input,
		registry: p.registry,
		Def:      def,
	}
	opt.SetOutput(io.Discard)
	if fs, ok := cmd.(FlagSetter); ok {
		fs.SetFlags(opt.FlagSet)
	}
	return cmd.Run(opt)
}

// Run is like RunWithInput by assuming that the first argument to the program
// is the given name, and passing the remaining arguments to the subcommand. If
// there are no arguments, then the subcommand runs without arguments.
func (p *Program) Run(name string) error {
	input := p.Input
	if len(input.Arguments) > 0 {
		input.Arguments = input.Arguments[1:]
	}
	return p.RunWithInput(name, input)
}

// UnknownCommand indicates an unknown subcommand was received.
type UnknownCommand struct {
	Name string
}

func (err UnknownCommand) Error() string {
	return fmt.Sprintf("unknown command %q", err.Name)
}

// Main provides a convenient entrypoint to the program.
//
// If the program has at least one argument, the subcommand corresponding to the
// first argument is run with the remaining arguments. If the first argument is
// not a registered subcommand, then an error is printed to Stderr, along with
// the global usage message.
//
// If the program has no arguments, then Main runs the "help" subcommand. If no
// help subcommand has been registered, then the global usage message is written
// to Stderr.
//
// If a subcommand returns an error, then the error is printed to Stderr. If the
// error is flag.ErrHelp, then a usage message of the command is written to
// Stderr instead.
func (p *Program) Main() {
	if len(p.Arguments) == 0 {
		if !p.Has("help") {
			p.WriteUsage(p.Stderr)
			return
		}
		p.run("help")
		return
	}
	subcommand := p.Arguments[0]
	if !p.Has(subcommand) {
		fmt.Fprintln(p.Stderr, UnknownCommand{Name: subcommand}.Error())
		p.WriteUsage(p.Stderr)
		return
	}
	p.run(subcommand)
}

// run runs a subcommand, and prints any resulting errors.
func (p *Program) run(subcommand string) {
	err := p.Run(subcommand)
	if err == nil {
		return
	}
	if err == flag.ErrHelp {
		p.WriteUsageOf(p.Stderr, p.Get(subcommand))
		return
	}
	fmt.Fprintln(p.Stderr, err)
}

// Input contains inputs to a program or subcommand.
type Input struct {
	// Program is the name of the program (e.g. os.Args[0]).
	Program string

	// Arguments are the arguments passed to the program (e.g. os.Args[1:]), or
	// to the subcommand.
	Arguments []string

	// Stdin is the file handle to be used as standard input.
	Stdin ReadFile

	// Stdout is the file handle to be used as standard output.
	Stdout WriteFile

	// Stderr is the file handle to be used as standard error.
	Stderr WriteFile

	// GlobalUsage specifies a description for the entire program. It is
	// expected to be a format string, where the first argument is the program
	// name, and the second argument is a list of subcommand summaries. If
	// empty, a default message is displayed.
	GlobalUsage string
}

// WriteUsageOf writes to w (or Stderr if w is nil) the usage of the given
// command definition.
func (i Input) WriteUsageOf(w io.Writer, def Def) {
	if w == nil {
		if w = i.Stderr; w == nil {
			return
		}
	}
	if def.Arguments == "" {
		fmt.Fprintf(w, "Usage: %s %s\n", i.Program, def.Name)
	} else {
		args := formatDesc(def.Arguments)
		fmt.Fprintf(w, "Usage: %s %s %s\n", i.Program, def.Name, args)
	}
	if def.Description != "" {
		desc := formatDesc(def.Description)
		fmt.Fprintf(w, "\n%s\n", desc)
	}
	if fs, ok := def.New().(FlagSetter); ok {
		flags := flag.NewFlagSet("", flag.ContinueOnError)
		fs.SetFlags(flags)
		flags.SetOutput(w)
		fmt.Fprintf(w, "\nFlags:\n")
		flags.PrintDefaults()
		fmt.Fprintf(w, "\n")
	}
}

// FileReader represents a file that can be read from.
type ReadFile = fs.File

// FileWriter represents a file that can be written to.
type WriteFile interface {
	fs.File
	io.Writer
}

type registry map[string]Def

// Has returns whether name is a registered subcommand.
func (r registry) Has(name string) bool {
	_, ok := r[name]
	return ok
}

// Get returns the definition of the subcommand mapped to the given name.
func (r registry) Get(name string) Def {
	return r[name]
}

// List returns a list of subcommand definitions, sorted by name.
func (r registry) List() []Def {
	list := make([]Def, 0, len(r))
	for _, def := range r {
		list = append(list, def)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list
}

// WriteSummary writes to w a list of each registered subcommand and its
// summary.
func (r registry) WriteSummary(w io.Writer) {
	if w == nil {
		return
	}
	//TODO: Receive width to improve display.
	list := r.List()
	nameWidth := 0
	for _, def := range list {
		if len(def.Name) > nameWidth {
			nameWidth = len(def.Name)
		}
	}
	for _, def := range list {
		fmt.Fprintf(w, "\t%-*s    %s\n", nameWidth, def.Name, def.Summary)
	}
}

// Command is an instance of a subcommand definition.
//
// Command may optionally implement FlagSetter, if it has flags.
type Command interface {
	// Run executes the subcommand.
	Run(Options) error
}

// Def describes a subcommand.
type Def struct {
	// Name is the name of the subcommand.
	Name string

	// Summary is a short description of the subcommand.
	Summary string

	// Arguments describes the arguments passed to the subcommand. When
	// displayed, it is prefixed with the name of the program, then the name of
	// the subcommand. ("program subcommand <arguments>")
	Arguments string

	// Description is a detailed description of the subcommand.
	Description string

	// New returns a new instance of the command.
	New func() Command
}

// FlagSetter is implemented by any type that can define flags on a FlagSet.
type FlagSetter interface {
	SetFlags(FlagSet)
}

// FlagSet is used to define flags. Each method corresponds to the method in
// flag.FlagSet.
type FlagSet interface {
	Bool(name string, value bool, usage string) *bool
	BoolVar(p *bool, name string, value bool, usage string)
	Duration(name string, value time.Duration, usage string) *time.Duration
	DurationVar(p *time.Duration, name string, value time.Duration, usage string)
	Float64(name string, value float64, usage string) *float64
	Float64Var(p *float64, name string, value float64, usage string)
	Func(name, usage string, fn func(string) error)
	Int(name string, value int, usage string) *int
	Int64(name string, value int64, usage string) *int64
	Int64Var(p *int64, name string, value int64, usage string)
	IntVar(p *int, name string, value int, usage string)
	String(name string, value string, usage string) *string
	StringVar(p *string, name string, value string, usage string)
	Uint(name string, value uint, usage string) *uint
	Uint64(name string, value uint64, usage string) *uint64
	Uint64Var(p *uint64, name string, value uint64, usage string)
	UintVar(p *uint, name string, value uint, usage string)
	Var(value flag.Value, name string, usage string)
}

// Options contains input and flags passed to a subcommand.
type Options struct {
	// FlagSet is an embedded set of flags for the subcommand.
	*flag.FlagSet

	// Input contains the inputs to the subcommand, with the fields inherited
	// from Program. The Arguments field is the unprocessed arguments after the
	// subcommand name.
	Input

	// Def is the definition of the running command.
	Def Def

	registry
}

// formatDesc formats a command description for readability.
func formatDesc(s string) string {
	s = strings.TrimSpace(s)
	//TODO: Wrap to n characters.
	return s
}

// ParseFlags parses the embedded FlagSet using opt.Arguments.
func (opt Options) ParseFlags() error {
	return opt.FlagSet.Parse(opt.Arguments)
}

// WriteGlobalUsage writes the GlobalUsage message to w, or Stderr if w is nil.
func (opt Options) WriteGlobalUsage(w io.Writer) {
	writeUsage(w, opt.Input, opt.registry)
}

// helpDef is the definition for the default help command.
var helpDef = Def{
	Name:        "help",
	Summary:     "Display help.",
	Arguments:   "[command]",
	Description: "Displays help for a command, or general help if no command is given.",
	New:         func() Command { return helpCommand{} },
}

// helpCommand is the implementation of the default help command.
type helpCommand struct{}

func (helpCommand) Run(opt Options) error {
	if err := opt.ParseFlags(); err != nil {
		return err
	}
	if name := opt.Arg(0); name != "" {
		if opt.Has(name) {
			opt.WriteUsageOf(opt.Stderr, opt.Get(name))
		} else {
			fmt.Fprintln(opt.Stderr, UnknownCommand{Name: name}.Error())
			fmt.Fprintln(opt.Stderr, "The following commands are available:")
			opt.WriteSummary(opt.Stderr)
		}
		return nil
	}
	opt.WriteGlobalUsage(opt.Stderr)
	return nil
}
