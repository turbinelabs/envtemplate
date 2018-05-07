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
	"io/ioutil"
	"os"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/turbinelabs/cli/command"
	"github.com/turbinelabs/test/assert"
	"github.com/turbinelabs/test/tempfile"

	tbnos "github.com/turbinelabs/nonstdlib/os"
)

func TestCLI(t *testing.T) {
	assert.Nil(t, mkCLI().Validate())
}

func mkMockOs(
	t testing.TB,
	in string,
	out *bytes.Buffer,
) (*tbnos.MockOS, func()) {
	ctrl := gomock.NewController(assert.Tracing(t))
	mockOS := tbnos.NewMockOS(ctrl)
	mockOS.EXPECT().Stdin().Return(bytes.NewBuffer([]byte(in)))
	if out != nil {
		mockOS.EXPECT().Stdout().Return(out)
	}
	return mockOS, ctrl.Finish
}

func TestRunIllegalFunc(t *testing.T) {
	c := cmd()
	err := c.Flags.Parse([]string{"-vars", "a-b=c"})
	assert.Nil(t, err)
	got := c.Runner.Run(c, nil)
	assert.Equal(t, got, c.BadInput(`Invalid template variable name: "a-b"`))
}

func TestRunDuplicatePredefFunc(t *testing.T) {
	c := cmd()
	err := c.Flags.Parse([]string{"-vars", "env=vne"})
	assert.Nil(t, err)
	got := c.Runner.Run(c, nil)
	assert.Equal(t, got, c.BadInput(`"env" cannot be used as a variable name`))
}

func TestRunDuplicateFunc(t *testing.T) {
	c := cmd()
	err := c.Flags.Parse([]string{"-vars", "foo=bar,foo=baz"})
	assert.Nil(t, err)
	got := c.Runner.Run(c, nil)
	assert.Equal(t, got, c.BadInput(`variable "foo" specified more than once`))
}

func TestRunNoop(t *testing.T) {
	out := &bytes.Buffer{}
	mockOS, finish := mkMockOs(t, "foo", out)
	defer finish()

	c := cmd()
	r := c.Runner.(*runner)
	r.os = mockOS

	got := r.Run(c, nil)
	assert.Equal(t, got, command.NoError())
	assert.Equal(t, "foo", out.String())
}

func TestRunBadTemplate(t *testing.T) {
	mockOS, finish := mkMockOs(t, "foo{{", nil)
	defer finish()

	c := cmd()
	r := c.Runner.(*runner)
	r.os = mockOS

	got := r.Run(c, nil)
	assert.Equal(t, got, c.Error("template: :1: unexpected unclosed action in command"))
}

func TestRunBadVariable(t *testing.T) {
	mockOS, finish := mkMockOs(t, "foo{{bar}}", nil)
	defer finish()

	c := cmd()
	r := c.Runner.(*runner)
	r.os = mockOS

	got := r.Run(c, nil)
	assert.Equal(t, got, c.Error(`template: :1: function "bar" not defined`))
}

func TestRunGoodVariable(t *testing.T) {
	out := &bytes.Buffer{}
	mockOS, finish := mkMockOs(t, "foo{{bar}}", out)
	defer finish()

	c := cmd()
	r := c.Runner.(*runner)
	r.os = mockOS

	err := c.Flags.Parse([]string{"-vars", "bar=BAR"})
	assert.Nil(t, err)

	got := r.Run(c, nil)
	assert.Equal(t, got, command.NoError())
	assert.Equal(t, "fooBAR", out.String())
}

func TestRunRequiredEnvMissing(t *testing.T) {
	mockOS, finish := mkMockOs(t, `foo{{env "BAR"}}`, nil)
	defer finish()

	mockOS.EXPECT().LookupEnv("BAR").Return("", false)

	c := cmd()
	r := c.Runner.(*runner)
	r.os = mockOS

	err := c.Flags.Parse([]string{"-vars", "bar=BAR"})
	assert.Nil(t, err)

	got := r.Run(c, nil)
	assert.Equal(t, got, c.Error(`template: :1:5: executing "" at <env "BAR">: error calling env: no value for $BAR in environment`))
}

func TestRunRequiredEnv(t *testing.T) {
	out := &bytes.Buffer{}
	mockOS, finish := mkMockOs(t, `foo{{env "BAR"}}`, out)
	defer finish()

	mockOS.EXPECT().LookupEnv("BAR").Return("baz", true)

	c := cmd()
	r := c.Runner.(*runner)
	r.os = mockOS

	err := c.Flags.Parse([]string{"-vars", "bar=BAR"})
	assert.Nil(t, err)

	got := r.Run(c, nil)
	assert.Equal(t, got, command.NoError())
	assert.Equal(t, "foobaz", out.String())
}

func TestRunOptionalEnvMissing(t *testing.T) {
	out := &bytes.Buffer{}
	mockOS, finish := mkMockOs(t, `foo{{envOrDefault "BAR" "$BAZ"}}`, out)
	defer finish()

	mockOS.EXPECT().LookupEnv("BAR").Return("", false)
	mockOS.EXPECT().ExpandEnv("$BAZ").Return("baz")

	c := cmd()
	r := c.Runner.(*runner)
	r.os = mockOS

	err := c.Flags.Parse([]string{"-vars", "bar=BAR"})
	assert.Nil(t, err)

	got := r.Run(c, nil)
	assert.Equal(t, got, command.NoError())
	assert.Equal(t, "foobaz", out.String())
}

func TestRunOptionalEnv(t *testing.T) {
	out := &bytes.Buffer{}
	mockOS, finish := mkMockOs(t, `foo{{envOrDefault "BAR" "$BAZ"}}`, out)
	defer finish()

	mockOS.EXPECT().LookupEnv("BAR").Return("blegga", true)

	c := cmd()
	r := c.Runner.(*runner)
	r.os = mockOS

	err := c.Flags.Parse([]string{"-vars", "bar=BAR"})
	assert.Nil(t, err)

	got := r.Run(c, nil)
	assert.Equal(t, got, command.NoError())
	assert.Equal(t, "fooblegga", out.String())
}

func TestRunDifferentFiles(t *testing.T) {
	in, removeIn := tempfile.Write(t, "foo{{bar}}")
	defer removeIn()
	out, removeOut := tempfile.Make(t)
	defer removeOut()

	c := cmd()
	err := c.Flags.Parse([]string{"-in", in, "-out", out, "-vars", "bar=baz"})
	assert.Nil(t, err)
	got := c.Runner.Run(c, nil)
	assert.Equal(t, got, command.NoError())

	gotIn, err := ioutil.ReadFile(in)
	assert.Nil(t, err)
	assert.Equal(t, string(gotIn), "foo{{bar}}")

	gotOut, err := ioutil.ReadFile(out)
	assert.Nil(t, err)
	assert.Equal(t, string(gotOut), "foobaz")
}

func TestRunSameFile(t *testing.T) {
	in, removeIn := tempfile.Write(t, "foo{{bar}}")
	defer removeIn()
	defer os.Remove(in + ".bak")

	c := cmd()
	err := c.Flags.Parse([]string{"-in", in, "-out", in, "-vars", "bar=baz"})
	assert.Nil(t, err)
	got := c.Runner.Run(c, nil)
	assert.Equal(t, got, command.NoError())

	gotIn, err := ioutil.ReadFile(in)
	assert.Nil(t, err)
	assert.Equal(t, string(gotIn), "foobaz")

	gotBak, err := ioutil.ReadFile(in + ".bak")
	assert.Nil(t, err)
	assert.Equal(t, string(gotBak), "foo{{bar}}")
}

func TestRunSameFileNoBackup(t *testing.T) {
	in, removeIn := tempfile.Write(t, "foo{{bar}}")
	defer removeIn()
	defer os.Remove(in + ".bak")

	c := cmd()
	err := c.Flags.Parse([]string{"-in", in, "-out", in, "-vars", "bar=baz", "-no-backup"})
	assert.Nil(t, err)
	got := c.Runner.Run(c, nil)
	assert.Equal(t, got, command.NoError())

	gotIn, err := ioutil.ReadFile(in)
	assert.Nil(t, err)
	assert.Equal(t, string(gotIn), "foobaz")

	_, err = os.Stat(in + ".bak")
	assert.True(t, os.IsNotExist(err))
}
