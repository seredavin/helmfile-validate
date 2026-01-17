package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestExtractFunctions(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "simple function call",
			content:  `{{ toYaml .Values }}`,
			expected: []string{"toYaml"},
		},
		{
			name:     "pipeline function",
			content:  `{{ .Values | toYaml | nindent 4 }}`,
			expected: []string{"toYaml", "nindent"},
		},
		{
			name:     "multiple functions",
			content:  `{{ exec "cmd" (list "arg1" "arg2") | trim }}`,
			expected: []string{"exec", "list", "trim"},
		},
		{
			name:     "function with variable assignment",
			content:  `{{ $result := exec "cmd" (list "arg") }}`,
			expected: []string{"exec", "list"},
		},
		{
			name:     "nested functions",
			content:  `{{ get "key" (fromYaml (readFile "file.yaml")) }}`,
			expected: []string{"get", "fromYaml", "readFile"},
		},
		{
			name:     "template comment should be ignored",
			content:  `{{/* this is a comment with exec and readFile */}}`,
			expected: []string{},
		},
		{
			name:     "if statement with function",
			content:  `{{ if isFile "path" }}exists{{ end }}`,
			expected: []string{"isFile"},
		},
		{
			name:     "range with function",
			content:  `{{ range readDir "." }}{{ . }}{{ end }}`,
			expected: []string{"readDir"},
		},
		{
			name:     "whitespace trimming markers",
			content:  `{{- toYaml .Values -}}`,
			expected: []string{"toYaml"},
		},
		{
			name:     "default function",
			content:  `{{ default "value" .Value }}`,
			expected: []string{"default"},
		},
		{
			name:     "requiredEnv function",
			content:  `{{ requiredEnv "MY_VAR" }}`,
			expected: []string{"requiredEnv"},
		},
		{
			name:     "complex template",
			content: `
releases:
  - name: {{ .Release.Name }}
    values:
      - {{ toYaml .Values | nindent 8 }}
      - config: {{ readFile "config.yaml" | fromYaml | toYaml | nindent 10 }}
    {{- if isFile "extra.yaml" }}
      - {{ readFile "extra.yaml" }}
    {{- end }}
`,
			expected: []string{"toYaml", "nindent", "readFile", "fromYaml", "isFile"},
		},
		{
			name:     "sprig functions",
			content:  `{{ .Name | quote | upper | trim }}`,
			expected: []string{"quote", "upper", "trim"},
		},
		{
			name:     "dict function",
			content:  `{{ $data := dict "key" "value" }}`,
			expected: []string{"dict"},
		},
		{
			name:     "include function",
			content:  `{{ include "mytemplate" . }}`,
			expected: []string{"include"},
		},
		{
			name:     "tpl function",
			content:  `{{ tpl "{{ .Name }}" . }}`,
			expected: []string{"tpl"},
		},
		{
			name:     "no functions - only field access",
			content:  `{{ .Values.key }}`,
			expected: []string{},
		},
		{
			name:     "no functions - only variable",
			content:  `{{ $var }}`,
			expected: []string{},
		},
		{
			name:     "keywords should be excluded",
			content:  `{{ if eq .Value "test" }}{{ range .Items }}{{ end }}{{ end }}`,
			expected: []string{},
		},
		{
			name:     "envExec function",
			content:  `{{ envExec (dict "VAR" "value") "cmd" (list "arg") }}`,
			expected: []string{"envExec", "dict", "list"},
		},
		{
			name:     "setValueAtPath function",
			content:  `{{ setValueAtPath "a.b.c" "value" .Values }}`,
			expected: []string{"setValueAtPath"},
		},
		{
			name:     "getOrNil function",
			content:  `{{ getOrNil "key.path" .Values }}`,
			expected: []string{"getOrNil"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFunctions(tt.content)
			sort.Strings(result)
			sort.Strings(tt.expected)

			if len(result) != len(tt.expected) {
				t.Errorf("extractFunctions() got %d functions %v, want %d functions %v",
					len(result), result, len(tt.expected), tt.expected)
				return
			}

			for i, fn := range result {
				if fn != tt.expected[i] {
					t.Errorf("extractFunctions() got %v, want %v", result, tt.expected)
					return
				}
			}
		})
	}
}

func TestContainsTemplateSyntax(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "has template syntax",
			content:  `key: {{ .Value }}`,
			expected: true,
		},
		{
			name:     "no template syntax",
			content:  `key: value`,
			expected: false,
		},
		{
			name:     "only opening braces",
			content:  `key: {{ value`,
			expected: false,
		},
		{
			name:     "only closing braces",
			content:  `key: value }}`,
			expected: false,
		},
		{
			name:     "empty content",
			content:  ``,
			expected: false,
		},
		{
			name:     "template comment",
			content:  `{{/* comment */}}`,
			expected: true,
		},
		{
			name:     "whitespace trimming",
			content:  `{{- .Value -}}`,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsTemplateSyntax(tt.content)
			if result != tt.expected {
				t.Errorf("containsTemplateSyntax() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		item     string
		expected bool
	}{
		{
			name:     "item exists",
			slice:    []string{"a", "b", "c"},
			item:     "b",
			expected: true,
		},
		{
			name:     "item does not exist",
			slice:    []string{"a", "b", "c"},
			item:     "d",
			expected: false,
		},
		{
			name:     "empty slice",
			slice:    []string{},
			item:     "a",
			expected: false,
		},
		{
			name:     "single item match",
			slice:    []string{"a"},
			item:     "a",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.slice, tt.item)
			if result != tt.expected {
				t.Errorf("contains() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFilterFunctions(t *testing.T) {
	funcs := []*FunctionUsage{
		{Name: "exec", Category: "helmfile"},
		{Name: "envExec", Category: "helmfile"},
		{Name: "readFile", Category: "helmfile"},
		{Name: "toYaml", Category: "helmfile"},
	}

	tests := []struct {
		name     string
		funcs    []*FunctionUsage
		names    []string
		expected int
	}{
		{
			name:     "filter exec functions",
			funcs:    funcs,
			names:    []string{"exec", "envExec"},
			expected: 2,
		},
		{
			name:     "filter single function",
			funcs:    funcs,
			names:    []string{"readFile"},
			expected: 1,
		},
		{
			name:     "filter non-existent function",
			funcs:    funcs,
			names:    []string{"nonexistent"},
			expected: 0,
		},
		{
			name:     "empty filter list",
			funcs:    funcs,
			names:    []string{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterFunctions(tt.funcs, tt.names)
			if len(result) != tt.expected {
				t.Errorf("filterFunctions() returned %d items, want %d", len(result), tt.expected)
			}
		})
	}
}

func TestFindTemplateFiles(t *testing.T) {
	// Create temporary directory structure for testing
	tmpDir, err := os.MkdirTemp("", "helmfile-validate-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files
	testFiles := map[string]string{
		"helmfile.yaml":              `releases: {{ .Values }}`,
		"helmfile.yaml.gotmpl":       `releases: {{ toYaml .Values }}`,
		"values.yaml":                `key: {{ .Value }}`,
		"values-prod.yaml":           `key: {{ .Value }}`,
		"templates/app.yaml.gotmpl":  `name: {{ .Name }}`,
		"templates/_helpers.tpl":     `{{- define "helper" }}{{ end }}`,
		"plain.yaml":                 `key: value`,
		".hidden/secret.yaml.gotmpl": `secret: {{ .Secret }}`,
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(tmpDir, path)
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write file %s: %v", path, err)
		}
	}

	files, err := findTemplateFiles(tmpDir)
	if err != nil {
		t.Fatalf("findTemplateFiles() error = %v", err)
	}

	// Expected files (should not include plain.yaml without templates or hidden files)
	expectedCount := 6 // helmfile.yaml, helmfile.yaml.gotmpl, values.yaml, values-prod.yaml, app.yaml.gotmpl, _helpers.tpl
	if len(files) != expectedCount {
		t.Errorf("findTemplateFiles() found %d files, want %d", len(files), expectedCount)
		for _, f := range files {
			t.Logf("  Found: %s", f)
		}
	}

	// Check that hidden directory is skipped
	for _, f := range files {
		if filepath.Base(filepath.Dir(f)) == ".hidden" {
			t.Errorf("findTemplateFiles() should skip hidden directories, found: %s", f)
		}
	}
}

func TestScanDirectory(t *testing.T) {
	// Create temporary directory structure for testing
	tmpDir, err := os.MkdirTemp("", "helmfile-validate-scan-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test helmfile content
	helmfileContent := `
releases:
  - name: {{ .Release.Name }}
    chart: {{ .Chart }}
    values:
      - {{ toYaml .Values | nindent 8 }}
      - config: {{ readFile "config.yaml" | fromYaml | quote }}
    {{- if isFile "extra.yaml" }}
    set:
      - name: extra
        value: {{ exec "cat" (list "extra.yaml") | trim }}
    {{- end }}
`

	valuesContent := `
key: {{ default "default" .Value }}
list: {{ list "a" "b" "c" | join "," }}
`

	testFiles := map[string]string{
		"helmfile.yaml": helmfileContent,
		"values.yaml":   valuesContent,
	}

	for path, content := range testFiles {
		fullPath := filepath.Join(tmpDir, path)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write file %s: %v", path, err)
		}
	}

	result := scanDirectory(tmpDir)

	// Check files scanned
	if len(result.FilesScanned) != 2 {
		t.Errorf("scanDirectory() scanned %d files, want 2", len(result.FilesScanned))
	}

	// Check helmfile functions found
	helmfileFuncNames := make(map[string]bool)
	for _, f := range result.HelmfileFunctions {
		helmfileFuncNames[f.Name] = true
	}

	expectedHelmfileFuncs := []string{"toYaml", "readFile", "fromYaml", "isFile", "exec"}
	for _, name := range expectedHelmfileFuncs {
		if !helmfileFuncNames[name] {
			t.Errorf("scanDirectory() missing helmfile function: %s", name)
		}
	}

	// Check sprig functions found
	sprigFuncNames := make(map[string]bool)
	for _, f := range result.SprigFunctions {
		sprigFuncNames[f.Name] = true
	}

	expectedSprigFuncs := []string{"nindent", "quote", "trim", "default", "list", "join"}
	for _, name := range expectedSprigFuncs {
		if !sprigFuncNames[name] {
			t.Errorf("scanDirectory() missing sprig function: %s", name)
		}
	}
}

func TestExtractFunctionsEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:     "multiline template",
			content:  "{{ toYaml\n  .Values\n}}",
			expected: []string{"toYaml"},
		},
		{
			name: "multiple templates on same line",
			content: `{{ .Name }}{{ toYaml .Values }}{{ quote .Value }}`,
			expected: []string{"toYaml", "quote"},
		},
		{
			name:     "function with string containing braces",
			content:  `{{ exec "echo" (list "{{test}}") }}`,
			expected: []string{"exec", "list"},
		},
		{
			name:     "ternary operator",
			content:  `{{ ternary "yes" "no" .Bool }}`,
			expected: []string{"ternary"},
		},
		{
			name:     "coalesce function",
			content:  `{{ coalesce .Value1 .Value2 "default" }}`,
			expected: []string{"coalesce"},
		},
		{
			name:     "empty function",
			content:  `{{ empty .Value }}`,
			expected: []string{"empty"},
		},
		{
			name:     "required function",
			content:  `{{ required "value is required" .Value }}`,
			expected: []string{"required"},
		},
		{
			name:     "fail function",
			content:  `{{ fail "error message" }}`,
			expected: []string{"fail"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractFunctions(tt.content)
			sort.Strings(result)
			sort.Strings(tt.expected)

			if len(result) != len(tt.expected) {
				t.Errorf("extractFunctions() got %d functions %v, want %d functions %v",
					len(result), result, len(tt.expected), tt.expected)
				return
			}

			for i, fn := range result {
				if fn != tt.expected[i] {
					t.Errorf("extractFunctions() got %v, want %v", result, tt.expected)
					return
				}
			}
		})
	}
}

func TestScanResultCategories(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "helmfile-validate-category-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Content with unknown function
	content := `
{{ customFunc .Value }}
{{ toYaml .Values }}
{{ default "x" .V }}
`

	if err := os.WriteFile(filepath.Join(tmpDir, "helmfile.yaml"), []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}

	result := scanDirectory(tmpDir)

	// Check that customFunc is in unknown category
	foundUnknown := false
	for _, f := range result.UnknownFunctions {
		if f.Name == "customFunc" {
			foundUnknown = true
			if f.Category != "unknown" {
				t.Errorf("customFunc should have category 'unknown', got '%s'", f.Category)
			}
			if f.IsKnown {
				t.Errorf("customFunc should have IsKnown=false")
			}
			break
		}
	}

	if !foundUnknown {
		t.Errorf("customFunc should be in UnknownFunctions")
	}

	// Check toYaml is in helmfile category
	foundHelmfile := false
	for _, f := range result.HelmfileFunctions {
		if f.Name == "toYaml" {
			foundHelmfile = true
			if f.Category != "helmfile" {
				t.Errorf("toYaml should have category 'helmfile', got '%s'", f.Category)
			}
			break
		}
	}

	if !foundHelmfile {
		t.Errorf("toYaml should be in HelmfileFunctions")
	}

	// Check default is in sprig category
	foundSprig := false
	for _, f := range result.SprigFunctions {
		if f.Name == "default" {
			foundSprig = true
			if f.Category != "sprig" {
				t.Errorf("default should have category 'sprig', got '%s'", f.Category)
			}
			break
		}
	}

	if !foundSprig {
		t.Errorf("default should be in SprigFunctions")
	}
}

func TestFunctionUsageCount(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "helmfile-validate-count-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create files with function usage
	// Note: extractFunctions returns unique function names per file,
	// so count represents the number of files containing the function
	file1 := `{{ toYaml .V1 }}{{ toYaml .V2 }}`
	file2 := `{{ toYaml .V3 }}`

	os.WriteFile(filepath.Join(tmpDir, "helmfile.yaml"), []byte(file1), 0644)
	os.WriteFile(filepath.Join(tmpDir, "values.yaml.gotmpl"), []byte(file2), 0644)

	result := scanDirectory(tmpDir)

	// Find toYaml usage
	var toYamlUsage *FunctionUsage
	for _, f := range result.HelmfileFunctions {
		if f.Name == "toYaml" {
			toYamlUsage = f
			break
		}
	}

	if toYamlUsage == nil {
		t.Fatal("toYaml not found in results")
	}

	// Count is the number of files containing the function (unique per file)
	if toYamlUsage.Count != 2 {
		t.Errorf("toYaml count = %d, want 2", toYamlUsage.Count)
	}

	if len(toYamlUsage.Files) != 2 {
		t.Errorf("toYaml files count = %d, want 2", len(toYamlUsage.Files))
	}
}

func TestParseList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: nil,
		},
		{
			name:     "single item",
			input:    "exec",
			expected: []string{"exec"},
		},
		{
			name:     "multiple items",
			input:    "exec,readFile,envExec",
			expected: []string{"exec", "readFile", "envExec"},
		},
		{
			name:     "items with spaces",
			input:    "exec, readFile , envExec",
			expected: []string{"exec", "readFile", "envExec"},
		},
		{
			name:     "empty items filtered",
			input:    "exec,,readFile,",
			expected: []string{"exec", "readFile"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseList(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("parseList(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("parseList(%q) = %v, want %v", tt.input, result, tt.expected)
					return
				}
			}
		})
	}
}

func TestValidateFunctionsBlacklist(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "helmfile-validate-blacklist-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	content := `{{ exec "cmd" (list "arg") }}{{ toYaml .Values }}{{ default "x" .V }}`
	os.WriteFile(filepath.Join(tmpDir, "helmfile.yaml"), []byte(content), 0644)

	result := scanDirectory(tmpDir)

	tests := []struct {
		name           string
		blacklist      string
		expectValid    bool
		expectViolates int
	}{
		{
			name:           "blacklist exec - should fail",
			blacklist:      "exec",
			expectValid:    false,
			expectViolates: 1,
		},
		{
			name:           "blacklist exec and toYaml - should fail with 2 violations",
			blacklist:      "exec,toYaml",
			expectValid:    false,
			expectViolates: 2,
		},
		{
			name:           "blacklist unused function - should pass",
			blacklist:      "readFile,envExec",
			expectValid:    true,
			expectViolates: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validation := validateFunctions(result, tt.blacklist, "")

			if validation.Valid != tt.expectValid {
				t.Errorf("validation.Valid = %v, want %v", validation.Valid, tt.expectValid)
			}

			if len(validation.Violations) != tt.expectViolates {
				t.Errorf("violations count = %d, want %d", len(validation.Violations), tt.expectViolates)
			}

			if validation.Mode != "blacklist" {
				t.Errorf("validation.Mode = %q, want %q", validation.Mode, "blacklist")
			}
		})
	}
}

func TestValidateFunctionsWhitelist(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "helmfile-validate-whitelist-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	content := `{{ exec "cmd" (list "arg") }}{{ toYaml .Values }}{{ default "x" .V }}`
	os.WriteFile(filepath.Join(tmpDir, "helmfile.yaml"), []byte(content), 0644)

	result := scanDirectory(tmpDir)

	tests := []struct {
		name           string
		whitelist      string
		expectValid    bool
		expectViolates int
	}{
		{
			name:           "whitelist all used functions - should pass",
			whitelist:      "exec,toYaml,default,list",
			expectValid:    true,
			expectViolates: 0,
		},
		{
			name:           "whitelist only some functions - should fail",
			whitelist:      "toYaml,default",
			expectValid:    false,
			expectViolates: 2, // exec and list not whitelisted
		},
		{
			name:           "whitelist none - should fail with all functions",
			whitelist:      "noop",
			expectValid:    false,
			expectViolates: 4, // all 4 functions violate
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validation := validateFunctions(result, "", tt.whitelist)

			if validation.Valid != tt.expectValid {
				t.Errorf("validation.Valid = %v, want %v", validation.Valid, tt.expectValid)
			}

			if len(validation.Violations) != tt.expectViolates {
				t.Errorf("violations count = %d, want %d (violations: %v)",
					len(validation.Violations), tt.expectViolates, getViolationNames(validation.Violations))
			}

			if validation.Mode != "whitelist" {
				t.Errorf("validation.Mode = %q, want %q", validation.Mode, "whitelist")
			}
		})
	}
}

func getViolationNames(violations []*FunctionUsage) []string {
	names := make([]string, len(violations))
	for i, v := range violations {
		names[i] = v.Name
	}
	return names
}

func TestValidationResultContainsFileInfo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "helmfile-validate-fileinfo-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create multiple files with exec
	file1 := `{{ exec "cmd1" (list) }}`
	file2 := `{{ exec "cmd2" (list) }}`

	os.WriteFile(filepath.Join(tmpDir, "helmfile.yaml"), []byte(file1), 0644)
	os.WriteFile(filepath.Join(tmpDir, "values.yaml.gotmpl"), []byte(file2), 0644)

	result := scanDirectory(tmpDir)
	validation := validateFunctions(result, "exec", "")

	if validation.Valid {
		t.Fatal("validation should have failed")
	}

	if len(validation.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(validation.Violations))
	}

	execViolation := validation.Violations[0]
	if execViolation.Name != "exec" {
		t.Errorf("violation name = %q, want %q", execViolation.Name, "exec")
	}

	if len(execViolation.Files) != 2 {
		t.Errorf("violation files count = %d, want 2", len(execViolation.Files))
	}

	// Check that both files are listed
	fileMap := make(map[string]bool)
	for _, f := range execViolation.Files {
		fileMap[f] = true
	}

	if !fileMap["helmfile.yaml"] {
		t.Error("helmfile.yaml not in violation files")
	}
	if !fileMap["values.yaml.gotmpl"] {
		t.Error("values.yaml.gotmpl not in violation files")
	}
}
