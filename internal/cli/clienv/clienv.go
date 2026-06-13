package clienv

import "io"

type Runtime struct {
	Out    io.Writer
	Err    io.Writer
	Config Config
}

type Config struct {
	JSON bool
}

func New(out io.Writer, err io.Writer) *Runtime {
	return &Runtime{
		Out: out,
		Err: err,
	}
}
