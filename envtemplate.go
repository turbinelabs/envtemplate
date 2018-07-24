/*
Copyright 2018 Turbine Labs, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"text/template"

	"github.com/turbinelabs/cli"
	"github.com/turbinelabs/cli/command"
	tbnflag "github.com/turbinelabs/nonstdlib/flag"
	tbnos "github.com/turbinelabs/nonstdlib/os"
	tbnregexp "github.com/turbinelabs/nonstdlib/regexp"
	tbnstrings "github.com/turbinelabs/nonstdlib/strings"
)

const TbnPublicVersion = "0.18.2"

const (
	description = `
Process a go-templated file, using environment and command-line variables
for substitutions.

Two functions are made avaiable to the templates:

{{ul "env"}}: used to specify a required environment variable:
    {{print "{{env \"TBN_HOME\""}}"}}

{{ul "envOrDefault"}}: used to specify an optional environment variable,
with a default value, which can reference other environment variables:
    {{print "{{envOrDefault \"TBN_HOME\" \"~/$TBN_WORKSPACE/tbn\"}}"}}

Additional variable substitutions can be specified using the --var flag.
`

	varsDesc = `
Additional vars referenced by the template file. Values are in the format
` + "`name=value`" + `. Multiple values may be comma-separated or the flag may
be repeated.`
)

func cmd() *command.Cmd {
	r := &runner{os: tbnos.New(), vars: tbnflag.NewStrings()}

	cmd := &command.Cmd{
		Name:        "envtemplate",
		Summary:     "Process a go-templated config file",
		Usage:       "[OPTIONS] <filename>",
		Description: description,
		Runner:      r,
	}

	cmd.Flags.StringVar(
		&r.in,
		"in",
		"",
		"The input `filename`. If empty, input will be read from STDIN",
	)
	cmd.Flags.StringVar(
		&r.out,
		"out",
		"",
		"The output `filename`. If empty, output will be go to STDOUT",
	)
	cmd.Flags.BoolVar(
		&r.nobackup,
		"no-backup",
		false,
		"if true, in the special case where --in and --out are the same file, don't keep a backup of the input file.",
	)
	cmd.Flags.Var(&r.vars, "vars", varsDesc)

	return cmd
}

type runner struct {
	os       tbnos.OS
	in       string
	out      string
	nobackup bool
	vars     tbnflag.Strings
}

func (r *runner) Run(cmd *command.Cmd, args []string) command.CmdErr {
	funcs, err := r.mkFuncMap()
	if err != nil {
		return cmd.BadInput(err)
	}

	var in []byte

	if r.in == "" {
		in, err = ioutil.ReadAll(r.os.Stdin())
		if err != nil {
			return cmd.Error(err)
		}
	} else {
		in, err = ioutil.ReadFile(r.in)
		if err != nil {
			return cmd.Error(err)
		}
		// in the special case where input and output are the same file,
		// read the file into a string, and write a backup of the file
		if r.in == r.out && !r.nobackup {
			err = ioutil.WriteFile(r.in+".bak", in, 0644)
			if err != nil {
				return cmd.Error(err)
			}
		}
	}

	tmpl, err := template.New("").Funcs(funcs).Parse(string(in))
	if err != nil {
		return cmd.Error(err)
	}

	out := &bytes.Buffer{}
	err = tmpl.Execute(out, nil)
	if err != nil {
		return cmd.Error(err)
	}

	if r.out == "" {
		fmt.Fprintf(r.os.Stdout(), out.String())
	} else {
		err = ioutil.WriteFile(r.out, out.Bytes(), 0644)
		if err != nil {
			return cmd.Error(err)
		}
	}

	return command.NoError()
}

func (r *runner) mkFuncMap() (template.FuncMap, error) {
	predef := template.FuncMap{
		"env":          r.env,
		"envOrDefault": r.envOrDefault,
	}

	funcs := template.FuncMap{
		"env":          r.env,
		"envOrDefault": r.envOrDefault,
	}

	for _, kvStr := range r.vars.Strings {
		name, value := tbnstrings.SplitFirstEqual(kvStr)

		if !tbnregexp.GolangIdentifierRegexp().MatchString(name) {
			return nil, fmt.Errorf("Invalid template variable name: %q", name)
		}

		if predef[name] != nil {
			return nil, fmt.Errorf("%q cannot be used as a variable name", name)
		}

		if funcs[name] != nil {
			return nil, fmt.Errorf("variable %q specified more than once", name)
		}

		funcs[name] = func() string { return value }
	}

	return funcs, nil
}

func (r *runner) env(key string) (string, error) {
	value, ok := r.os.LookupEnv(key)
	if !ok {
		return "", fmt.Errorf("no value for $%s in environment", key)
	}
	return value, nil
}

func (r *runner) envOrDefault(key, defValue string) string {
	value, ok := r.os.LookupEnv(key)
	if !ok {
		return r.os.ExpandEnv(defValue)
	}
	return value
}

func mkCLI() cli.CLI {
	return cli.New(TbnPublicVersion, cmd())
}

func main() {
	mkCLI().Main()
}
