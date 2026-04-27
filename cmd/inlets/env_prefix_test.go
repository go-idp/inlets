package main

import (
	"strings"
	"testing"

	"github.com/urfave/cli/v2"
)

// envVarsFromFlag returns EnvVars for urfave flag types that support them.
func envVarsFromFlag(f cli.Flag) []string {
	switch fl := f.(type) {
	case *cli.StringFlag:
		return fl.EnvVars
	case *cli.IntFlag:
		return fl.EnvVars
	case *cli.BoolFlag:
		return fl.EnvVars
	case *cli.StringSliceFlag:
		return fl.EnvVars
	case *cli.IntSliceFlag:
		return fl.EnvVars
	case *cli.Float64Flag:
		return fl.EnvVars
	case *cli.UintFlag:
		return fl.EnvVars
	case *cli.Uint64Flag:
		return fl.EnvVars
	default:
		return nil
	}
}

func assertFlagEnvVarsUseInletsPrefix(t *testing.T, path string, f cli.Flag) {
	t.Helper()
	for _, name := range envVarsFromFlag(f) {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, "INLETS_") {
			t.Errorf("%s: environment variable %q must use INLETS_ prefix", path, name)
		}
	}
}

func walkCommandTreeForEnvPrefix(t *testing.T, path string, cmd *cli.Command) {
	t.Helper()
	if cmd == nil {
		return
	}
	p := path
	if cmd.Name != "" {
		if path == "" {
			p = cmd.Name
		} else {
			p = path + " " + cmd.Name
		}
	}
	for _, f := range cmd.Flags {
		assertFlagEnvVarsUseInletsPrefix(t, p, f)
	}
	for _, sub := range cmd.Subcommands {
		walkCommandTreeForEnvPrefix(t, p, sub)
	}
}

func TestClientCommandEnvVarsUseINLETSPrefix(t *testing.T) {
	t.Parallel()
	walkCommandTreeForEnvPrefix(t, "", Client())
}

func TestServerCommandEnvVarsUseINLETSPrefix(t *testing.T) {
	t.Parallel()
	walkCommandTreeForEnvPrefix(t, "", Server())
}

func TestForwardCommandEnvVarsUseINLETSPrefix(t *testing.T) {
	t.Parallel()
	walkCommandTreeForEnvPrefix(t, "", Forward())
}
