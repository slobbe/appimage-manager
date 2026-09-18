package clienv

import (
	"io"

	"github.com/slobbe/appimage-manager/internal/cli/presentation"
)

type Runtime struct {
	Out    io.Writer
	Err    io.Writer
	Config Config
	Policy presentation.Policy
}

type Config struct {
	JSON           bool
	Yes            bool
	NonInteractive bool
	Color          string
}

func New(out io.Writer, err io.Writer) *Runtime {
	return &Runtime{
		Out:    out,
		Err:    err,
		Config: Config{Color: presentation.ColorAuto},
		Policy: presentation.NewPolicy(),
	}
}

func (r *Runtime) Success(w io.Writer, text string) string {
	return r.Policy.Success(w, r.Config.Color, r.Config.JSON, text)
}

func (r *Runtime) Emphasis(w io.Writer, text string) string {
	return r.Policy.Emphasis(w, r.Config.Color, r.Config.JSON, text)
}

func (r *Runtime) ActivityEnabled(w io.Writer) bool {
	return r.Policy.ActivityEnabled(w, r.Config.JSON)
}
