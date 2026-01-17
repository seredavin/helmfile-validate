package app

import (
	"github.com/seredavin/helmfile-validate/pkg/helmfile/state"
	"github.com/seredavin/helmfile-validate/pkg/helmfile/yaml"
)

type LoadOpts struct {
	Selectors   []string
	Environment state.SubhelmfileEnvironmentSpec

	RetainValuesFiles bool

	// CalleePath is the absolute path to the file being loaded
	CalleePath string

	Reverse bool

	Filter bool
}

func (o LoadOpts) DeepCopy() LoadOpts {
	bytes, err := yaml.Marshal(o)
	if err != nil {
		panic(err)
	}

	new := LoadOpts{}
	if err := yaml.Unmarshal(bytes, &new); err != nil {
		panic(err)
	}

	return new
}
