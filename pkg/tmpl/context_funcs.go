package tmpl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"

	"golang.org/x/sync/errgroup"

	"github.com/seredavin/helmfile-validate/pkg/maputil"
	"github.com/seredavin/helmfile-validate/pkg/yaml"
)

type Values = map[string]any

func (c *Context) createFuncMap() template.FuncMap {
	funcMap := template.FuncMap{
		"envExec":          c.EnvExec,
		"exec":             c.Exec,
		"isFile":           c.IsFile,
		"isDir":            c.IsDir,
		"readFile":         c.ReadFile,
		"readDir":          c.ReadDir,
		"readDirEntries":   c.ReadDirEntries,
		"toYaml":           ToYaml,
		"fromYaml":         FromYaml,
		"setValueAtPath":   SetValueAtPath,
		"requiredEnv":      RequiredEnv,
		"get":              get,
		"getOrNil":         getOrNil,
		"tpl":              c.Tpl,
		"required":         Required,
		"fetchSecretValue": fetchSecretValue,
		"expandSecretRefs": fetchSecretValues,
	}

	return funcMap
}

func (c *Context) EnvExec(envs map[string]any, command string, args []any, inputs ...string) (string, error) {
	var input string
	if len(inputs) > 0 {
		input = inputs[0]
	}

	strArgs := make([]string, len(args))
	for i, a := range args {
		switch a.(type) {
		case string:
			strArgs[i] = fmt.Sprintf("%v", a)
		default:
			return "", fmt.Errorf("unexpected type of arg \"%s\" in args %v at index %d", reflect.TypeOf(a), args, i)
		}
	}

	cmd := exec.Command(command, strArgs...)
	cmd.Dir = c.basePath
	if envs != nil {
		cmd.Env = os.Environ()
		for k, v := range envs {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	g := errgroup.Group{}

	if len(input) > 0 {
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return "", err
		}

		g.Go(func() error {
			defer func() {
				_ = stdin.Close()
			}()

			size := len(input)
			i := 0

			for {
				n, err := io.WriteString(stdin, input[i:])
				if err != nil {
					return fmt.Errorf("failed while writing %d bytes to stdin of \"%s\": %v", len(input), command, err)
				}

				i += n

				if i == size {
					return nil
				}
			}
		})
	}

	var bytes []byte

	g.Go(func() error {
		bs, err := cmd.CombinedOutput()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return fmt.Errorf("command %q exited with code %d: %s", command, exitErr.ExitCode(), string(bs))
			}
			return err
		}
		bytes = bs
		return nil
	})

	if err := g.Wait(); err != nil {
		return "", err
	}

	return string(bytes), nil
}

func (c *Context) Exec(command string, args []any, inputs ...string) (string, error) {
	return c.EnvExec(nil, command, args, inputs...)
}

func (c *Context) IsFile(filename string) (bool, error) {
	var path string
	if filepath.IsAbs(filename) {
		path = filename
	} else {
		path = filepath.Join(c.basePath, filename)
	}

	stat, err := os.Stat(path)
	if err == nil {
		return !stat.IsDir(), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (c *Context) IsDir(filename string) (bool, error) {
	var path string
	if filepath.IsAbs(filename) {
		path = filename
	} else {
		path = filepath.Join(c.basePath, filename)
	}

	stat, err := os.Stat(path)
	if err == nil {
		return stat.IsDir(), nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (c *Context) ReadFile(filename string) (string, error) {
	var path string
	if filepath.IsAbs(filename) {
		path = filename
	} else {
		path = filepath.Join(c.basePath, filename)
	}

	if c.fs.ReadFile == nil {
		return "", fmt.Errorf("readFile is not implemented")
	}

	bytes, err := c.fs.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (c *Context) ReadDir(path string) ([]string, error) {
	var contextPath string
	if filepath.IsAbs(path) {
		contextPath = path
	} else {
		contextPath = filepath.Join(c.basePath, path)
	}

	entries, err := c.fs.ReadDir(contextPath)
	if err != nil {
		return nil, fmt.Errorf("ReadDir %q: %w", contextPath, err)
	}

	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		paths = append(paths, filepath.Join(path, entry.Name()))
	}

	return paths, nil
}

func (c *Context) ReadDirEntries(path string) ([]fs.DirEntry, error) {
	var contextPath string
	if filepath.IsAbs(path) {
		contextPath = path
	} else {
		contextPath = filepath.Join(c.basePath, path)
	}
	entries, err := c.fs.ReadDir(contextPath)
	if err != nil {
		return nil, fmt.Errorf("ReadDirEntries %q: %w", contextPath, err)
	}
	return entries, nil
}

func (c *Context) Tpl(text string, data any) (string, error) {
	buf, err := c.RenderTemplateToBuffer(text, data)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

func ToYaml(v any) (string, error) {
	data, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func FromYaml(str string) (any, error) {
	var m any

	if err := yaml.Unmarshal([]byte(str), &m); err != nil {
		return nil, fmt.Errorf("%s, offending yaml: %s", err, str)
	}

	m, err := maputil.RecursivelyStringifyMapKey(m)
	if err != nil {
		return nil, fmt.Errorf("%s, offending yaml: %s", err, str)
	}

	return m, nil
}

func SetValueAtPath(path string, value any, values Values) (Values, error) {
	var current any
	current = values
	components := strings.Split(path, ".")
	pathToMap := components[:len(components)-1]
	key := components[len(components)-1]
	for _, k := range pathToMap {
		var elem any

		switch typedCurrent := current.(type) {
		case map[string]any:
			v, exists := typedCurrent[k]
			if !exists {
				return nil, fmt.Errorf("failed to set value at path \"%s\": value for key \"%s\" does not exist", path, k)
			}
			elem = v
		case map[any]any:
			v, exists := typedCurrent[k]
			if !exists {
				return nil, fmt.Errorf("failed to set value at path \"%s\": value for key \"%s\" does not exist", path, k)
			}
			elem = v
		default:
			return nil, fmt.Errorf("failed to set value at path \"%s\": value for key \"%s\" was not a map", path, k)
		}

		switch typedElem := elem.(type) {
		case map[string]any, map[any]any:
			current = typedElem
		default:
			return nil, fmt.Errorf("failed to set value at path \"%s\": value for key \"%s\" was not a map", path, k)
		}
	}

	switch typedCurrent := current.(type) {
	case map[string]any:
		typedCurrent[key] = value
	case map[any]any:
		typedCurrent[key] = value
	default:
		return nil, fmt.Errorf("failed to set value at path \"%s\": value for key \"%s\" was not a map", path, key)
	}
	return values, nil
}

func RequiredEnv(name string) (string, error) {
	if val, exists := os.LookupEnv(name); exists && len(val) > 0 {
		return val, nil
	}

	return "", fmt.Errorf("required env var `%s` is not set", name)
}

func Required(warn string, val any) (any, error) {
	if val == nil {
		return nil, fmt.Errorf("%s", warn)
	} else if _, ok := val.(string); ok {
		if val == "" {
			return nil, fmt.Errorf("%s", warn)
		}
	}

	return val, nil
}

// Placeholder functions - these would need actual implementation
func fetchSecretValue(key string) (string, error) {
	return "", fmt.Errorf("fetchSecretValue is not implemented")
}

func fetchSecretValues(input any) (any, error) {
	return input, nil
}

func get(path string, m map[string]any) (any, error) {
	keys := strings.Split(path, ".")
	var current any = m

	for _, key := range keys {
		switch typed := current.(type) {
		case map[string]any:
			val, ok := typed[key]
			if !ok {
				return nil, fmt.Errorf("key %q not found", key)
			}
			current = val
		case map[any]any:
			val, ok := typed[key]
			if !ok {
				return nil, fmt.Errorf("key %q not found", key)
			}
			current = val
		default:
			return nil, fmt.Errorf("cannot traverse non-map type %T", current)
		}
	}

	return current, nil
}

func getOrNil(path string, m map[string]any) any {
	val, _ := get(path, m)
	return val
}

// Suppress unused variable warning
var _ = context.Background
