# snek
[![Go Reference](https://pkg.go.dev/badge/github.com/anaminus/snek.svg)](https://pkg.go.dev/github.com/anaminus/snek)

**snek** is a Go package that implements lightweight subcommands for a
command-line application.

An example of a main entry point:

```go
var Program = snek.NewProgram("", os.Args)

func main() {
	Program.Main()
}
```

 An example of a subcommand:

```go
func init() {
	Program.Register(snek.Def{
		Name:        "echo",
		Summary:     "Display text.",
		Arguments:   "[-n] [TEXT...]",
		Description: `Write the given arguments to standard output.`,
		New:         func() snek.Command { return &EchoCommand{} },
	})
}

type EchoCommand struct {
	NoNewline bool
}

func (c *EchoCommand) SetFlags(flags snek.FlagSet) {
	flags.BoolVar(&c.NoNewline, "n", false, "Suppress trailing newline.")
}

func (c EchoCommand) Run(opt snek.Options) error {
	if err := opt.ParseFlags(); err != nil {
		return err
	}
	out := strings.Join(opt.Args(), " ")
	fmt.Print(out)
	if !c.NoNewline {
		fmt.Print("\n")
	}
	return nil
}
```
