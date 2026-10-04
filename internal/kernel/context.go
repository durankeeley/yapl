package kernel

import (
	"flag"
	"io"
)

// Context is what a command's Run function receives.
type Context struct {
	Args   []string
	Flags  *flag.FlagSet
	Stdout io.Writer
	Stderr io.Writer
	Kernel *Kernel
}

// Arg returns the i-th positional argument, or "" when absent.
func (c *Context) Arg(i int) string {
	if i < 0 || i >= len(c.Args) {
		return ""
	}
	return c.Args[i]
}

// String returns the value of a string flag declared by the command.
func (c *Context) String(name string) string {
	v, _ := c.get(name).(string)
	return v
}

// Bool returns the value of a bool flag declared by the command.
func (c *Context) Bool(name string) bool {
	v, _ := c.get(name).(bool)
	return v
}

// Int returns the value of an int flag declared by the command.
func (c *Context) Int(name string) int {
	v, _ := c.get(name).(int)
	return v
}

// IsSet reports whether the user passed the flag explicitly.
func (c *Context) IsSet(name string) bool {
	set := false
	if c.Flags != nil {
		c.Flags.Visit(func(f *flag.Flag) {
			if f.Name == name {
				set = true
			}
		})
	}
	return set
}

func (c *Context) get(name string) any {
	if c.Flags == nil {
		return nil
	}
	f := c.Flags.Lookup(name)
	if f == nil {
		return nil
	}
	if g, ok := f.Value.(flag.Getter); ok {
		return g.Get()
	}
	return nil
}
