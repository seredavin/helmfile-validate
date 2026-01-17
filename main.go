package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/fatih/color"

	"github.com/kirillseredavin/helmfile-validate/pkg/filesystem"
	"github.com/kirillseredavin/helmfile-validate/pkg/tmpl"
)

// FunctionUsage tracks where a function is used
type FunctionUsage struct {
	Name     string   `json:"name"`
	Files    []string `json:"files"`
	Count    int      `json:"count"`
	IsKnown  bool     `json:"is_known"`
	Category string   `json:"category"` // "helmfile", "sprig", "unknown"
}

// HookUsage tracks where hooks are defined
type HookUsage struct {
	File    string   `json:"file"`
	Release string   `json:"release,omitempty"`
	Events  []string `json:"events,omitempty"`
	Command string   `json:"command,omitempty"`
	Line    int      `json:"line,omitempty"`
}

// ScanResult contains the complete scan results
type ScanResult struct {
	Directory         string           `json:"directory"`
	FilesScanned      []string         `json:"files_scanned"`
	HelmfileFunctions []*FunctionUsage `json:"helmfile_functions"`
	SprigFunctions    []*FunctionUsage `json:"sprig_functions"`
	UnknownFunctions  []*FunctionUsage `json:"unknown_functions,omitempty"`
	Hooks             []*HookUsage     `json:"hooks,omitempty"`
}

// ValidationResult contains validation errors
type ValidationResult struct {
	Valid          bool             `json:"valid"`
	Violations     []*FunctionUsage `json:"violations,omitempty"`
	HookViolations []*HookUsage     `json:"hook_violations,omitempty"`
	Mode           string           `json:"mode"` // "blacklist", "whitelist", or "no-hooks"
	Rules          []string         `json:"rules,omitempty"`
}

// OutputResult combines scan and validation results for JSON output
type OutputResult struct {
	Scan       *ScanResult       `json:"scan"`
	Validation *ValidationResult `json:"validation,omitempty"`
}

var (
	jsonOutput    bool
	showExecOnly  bool
	showUnknown   bool
	showInsecure  bool
	listFunctions bool
	blacklist     string
	whitelist     string
	noColor       bool
	noHooks       bool
)

func init() {
	flag.BoolVar(&jsonOutput, "json", false, "Output results as JSON")
	flag.BoolVar(&showExecOnly, "exec", false, "Show only exec/envExec function usage")
	flag.BoolVar(&showUnknown, "unknown", false, "Show only unknown functions")
	flag.BoolVar(&showInsecure, "insecure", false, "Show potentially insecure functions (exec, readFile, etc.)")
	flag.BoolVar(&listFunctions, "list", false, "List all available template functions and exit")
	flag.StringVar(&blacklist, "blacklist", "", "Comma-separated list of forbidden functions (exit with error if found)")
	flag.StringVar(&whitelist, "whitelist", "", "Comma-separated list of allowed functions (exit with error if other functions found)")
	flag.BoolVar(&noColor, "no-color", false, "Disable colored output")
	flag.BoolVar(&noHooks, "no-hooks", false, "Forbid hooks in helmfile (exit with error if hooks are found)")
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `helmfile-validate - Scan helmfile templates for function usage

Usage:
  helmfile-validate [options] [directory]

Options:
`)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  helmfile-validate .                              # Scan current directory
  helmfile-validate /path/to/helmfile              # Scan specific directory
  helmfile-validate -json .                        # Output as JSON
  helmfile-validate -exec .                        # Show only exec/envExec usage
  helmfile-validate -insecure .                    # Show potentially insecure functions
  helmfile-validate -list                          # List all available functions
  helmfile-validate -blacklist "exec,envExec" .    # Fail if exec or envExec are used
  helmfile-validate -whitelist "toYaml,default" .  # Fail if any other functions are used
  helmfile-validate -no-hooks .                    # Fail if any hooks are defined

Validation modes:
  -blacklist: Specify functions that are NOT allowed. If any blacklisted function
              is found, the tool exits with code 1 and shows detailed error info.

  -whitelist: Specify functions that ARE allowed. If any function NOT in the
              whitelist is found, the tool exits with code 1 and shows detailed error info.

  -no-hooks:  Forbid hooks in helmfile. If any hooks are found, the tool exits
              with code 1 and shows where hooks are defined.

  Note: -blacklist and -whitelist are mutually exclusive. -no-hooks can be combined with them.
`)
	}

	flag.Parse()

	// Disable colors if requested or if not a terminal
	if noColor {
		color.NoColor = true
	}

	// Validate flags
	if blacklist != "" && whitelist != "" {
		fmt.Fprintf(os.Stderr, "Error: -blacklist and -whitelist are mutually exclusive\n")
		os.Exit(2)
	}

	// List all functions mode
	if listFunctions {
		listAllFunctions()
		return
	}

	// Get directory path from args or use current directory
	dirPath := "."
	if flag.NArg() > 0 {
		dirPath = flag.Arg(0)
	}

	// Resolve absolute path
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving path: %v\n", err)
		os.Exit(1)
	}

	// Check if directory exists
	info, err := os.Stat(absPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error accessing path: %v\n", err)
		os.Exit(1)
	}
	if !info.IsDir() {
		fmt.Fprintf(os.Stderr, "Path is not a directory: %s\n", absPath)
		os.Exit(1)
	}

	result := scanDirectory(absPath)

	// Perform validation if blacklist, whitelist, or no-hooks is specified
	var validation *ValidationResult
	if blacklist != "" || whitelist != "" {
		validation = validateFunctions(result, blacklist, whitelist)
	}

	// Validate hooks if no-hooks flag is set
	if noHooks {
		hookValidation := validateHooks(result)
		if validation == nil {
			validation = hookValidation
		} else {
			// Merge hook validation into existing validation
			if !hookValidation.Valid {
				validation.Valid = false
				validation.HookViolations = hookValidation.HookViolations
				if validation.Mode != "" {
					validation.Mode = validation.Mode + "+no-hooks"
				} else {
					validation.Mode = "no-hooks"
				}
			}
		}
	}

	if jsonOutput {
		outputJSONWithValidation(result, validation)
	} else {
		outputText(result, absPath)
		if validation != nil {
			outputValidation(validation)
		}
	}

	// Exit with error if validation failed
	if validation != nil && !validation.Valid {
		os.Exit(1)
	}
}

func parseList(list string) []string {
	if list == "" {
		return nil
	}
	parts := strings.Split(list, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

func validateFunctions(result *ScanResult, blacklistStr, whitelistStr string) *ValidationResult {
	validation := &ValidationResult{
		Valid:      true,
		Violations: []*FunctionUsage{},
	}

	// Collect all used functions
	allFuncs := make(map[string]*FunctionUsage)
	for _, f := range result.HelmfileFunctions {
		allFuncs[f.Name] = f
	}
	for _, f := range result.SprigFunctions {
		allFuncs[f.Name] = f
	}
	for _, f := range result.UnknownFunctions {
		allFuncs[f.Name] = f
	}

	if blacklistStr != "" {
		// Blacklist mode: fail if any blacklisted function is found
		validation.Mode = "blacklist"
		validation.Rules = parseList(blacklistStr)
		blacklistSet := make(map[string]bool)
		for _, name := range validation.Rules {
			blacklistSet[name] = true
		}

		for name, usage := range allFuncs {
			if blacklistSet[name] {
				validation.Valid = false
				validation.Violations = append(validation.Violations, usage)
			}
		}
	} else if whitelistStr != "" {
		// Whitelist mode: fail if any function not in whitelist is found
		validation.Mode = "whitelist"
		validation.Rules = parseList(whitelistStr)
		whitelistSet := make(map[string]bool)
		for _, name := range validation.Rules {
			whitelistSet[name] = true
		}

		for name, usage := range allFuncs {
			if !whitelistSet[name] {
				validation.Valid = false
				validation.Violations = append(validation.Violations, usage)
			}
		}
	}

	// Sort violations by name
	sort.Slice(validation.Violations, func(i, j int) bool {
		return validation.Violations[i].Name < validation.Violations[j].Name
	})

	return validation
}

func validateHooks(result *ScanResult) *ValidationResult {
	validation := &ValidationResult{
		Valid: true,
		Mode:  "no-hooks",
	}

	if len(result.Hooks) > 0 {
		validation.Valid = false
		validation.HookViolations = result.Hooks
	}

	return validation
}

func outputJSONWithValidation(result *ScanResult, validation *ValidationResult) {
	output := OutputResult{
		Scan:       result,
		Validation: validation,
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(output)
}

func outputValidation(validation *ValidationResult) {
	red := color.New(color.FgRed, color.Bold)
	yellow := color.New(color.FgYellow)
	cyan := color.New(color.FgCyan)

	fmt.Println()
	if validation.Valid {
		green := color.New(color.FgGreen, color.Bold)
		green.Println("=== Validation PASSED ===")
		fmt.Printf("Mode: %s\n", validation.Mode)
		fmt.Printf("Rules: %s\n", strings.Join(validation.Rules, ", "))
		return
	}

	red.Println("=== Validation FAILED ===")
	fmt.Println()

	if validation.Mode == "blacklist" {
		red.Printf("BLACKLISTED functions found!\n")
		fmt.Printf("Forbidden functions: %s\n\n", strings.Join(validation.Rules, ", "))
	} else {
		red.Printf("Functions NOT in WHITELIST found!\n")
		fmt.Printf("Allowed functions: %s\n\n", strings.Join(validation.Rules, ", "))
	}

	red.Printf("Violations (%d):\n", len(validation.Violations))
	fmt.Println()

	for _, v := range validation.Violations {
		yellow.Printf("  ✗ %s", v.Name)
		fmt.Printf(" (category: %s, used %d times)\n", v.Category, v.Count)
		fmt.Println("    Found in:")
		for _, file := range v.Files {
			cyan.Printf("      - %s\n", file)
		}
		fmt.Println()
	}

	red.Println("Please remove or replace the forbidden functions to pass validation.")
}

func scanDirectory(absPath string) *ScanResult {
	// Get available FuncMap
	r := tmpl.NewFileRenderer(filesystem.DefaultFileSystem(), absPath, nil)
	funcMap := r.Context.CreateFuncMap()

	// Build known functions map
	knownFuncs := make(map[string]string) // name -> category
	for name := range funcMap {
		knownFuncs[name] = "sprig"
	}

	// Mark helmfile-specific functions
	helmfileSpecific := []string{
		"envExec", "exec", "isFile", "isDir", "readFile", "readDir", "readDirEntries",
		"toYaml", "fromYaml", "setValueAtPath", "requiredEnv", "get", "getOrNil",
		"tpl", "required", "fetchSecretValue", "expandSecretRefs", "sprigGet", "include",
	}
	for _, name := range helmfileSpecific {
		knownFuncs[name] = "helmfile"
	}

	// Find all template files
	files, _ := findTemplateFiles(absPath)

	result := &ScanResult{
		Directory:    absPath,
		FilesScanned: []string{},
	}

	if len(files) == 0 {
		return result
	}

	// Scan files for function usage
	usageMap := make(map[string]*FunctionUsage)

	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		relPath, _ := filepath.Rel(absPath, file)
		result.FilesScanned = append(result.FilesScanned, relPath)
		funcs := extractFunctions(string(content))

		for _, funcName := range funcs {
			if usage, exists := usageMap[funcName]; exists {
				usage.Count++
				if !contains(usage.Files, relPath) {
					usage.Files = append(usage.Files, relPath)
				}
			} else {
				category, isKnown := knownFuncs[funcName]
				if !isKnown {
					category = "unknown"
				}
				usageMap[funcName] = &FunctionUsage{
					Name:     funcName,
					Files:    []string{relPath},
					Count:    1,
					IsKnown:  isKnown,
					Category: category,
				}
			}
		}
	}

	// Categorize results
	for _, usage := range usageMap {
		switch usage.Category {
		case "helmfile":
			result.HelmfileFunctions = append(result.HelmfileFunctions, usage)
		case "sprig":
			result.SprigFunctions = append(result.SprigFunctions, usage)
		default:
			result.UnknownFunctions = append(result.UnknownFunctions, usage)
		}
	}

	sortByName := func(funcs []*FunctionUsage) {
		sort.Slice(funcs, func(i, j int) bool {
			return funcs[i].Name < funcs[j].Name
		})
	}

	sortByName(result.HelmfileFunctions)
	sortByName(result.SprigFunctions)
	sortByName(result.UnknownFunctions)

	return result
}

func outputJSON(result *ScanResult) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(result)
}

func outputText(result *ScanResult, absPath string) {
	fmt.Printf("Scanning directory: %s\n\n", absPath)

	if len(result.FilesScanned) == 0 {
		fmt.Println("No helmfile template files found.")
		return
	}

	fmt.Printf("Found %d template files:\n", len(result.FilesScanned))
	for _, f := range result.FilesScanned {
		fmt.Printf("  - %s\n", f)
	}
	fmt.Println()

	// Filter based on flags
	helmfileFuncs := result.HelmfileFunctions
	sprigFuncs := result.SprigFunctions
	unknownFuncs := result.UnknownFunctions

	if showExecOnly {
		helmfileFuncs = filterFunctions(helmfileFuncs, []string{"exec", "envExec"})
		sprigFuncs = nil
		unknownFuncs = nil
	}

	if showInsecure {
		insecureFuncs := []string{"exec", "envExec", "readFile", "readDir", "readDirEntries"}
		helmfileFuncs = filterFunctions(helmfileFuncs, insecureFuncs)
		sprigFuncs = nil
		unknownFuncs = nil
	}

	if showUnknown {
		helmfileFuncs = nil
		sprigFuncs = nil
	}

	// Print results
	if !showUnknown && !showExecOnly && !showInsecure {
		totalUsed := len(helmfileFuncs) + len(sprigFuncs)
		fmt.Printf("=== Used Template Functions ===\n")
		fmt.Printf("Total known functions used: %d\n\n", totalUsed)
	}

	if len(helmfileFuncs) > 0 {
		fmt.Printf("--- Helmfile-specific functions (%d) ---\n", len(helmfileFuncs))
		for _, usage := range helmfileFuncs {
			fmt.Printf("  %s (used %d times)\n", usage.Name, usage.Count)
			for _, f := range usage.Files {
				fmt.Printf("    - %s\n", f)
			}
		}
		fmt.Println()
	}

	if len(sprigFuncs) > 0 {
		fmt.Printf("--- Sprig functions (%d) ---\n", len(sprigFuncs))
		for _, usage := range sprigFuncs {
			fmt.Printf("  %s (used %d times)\n", usage.Name, usage.Count)
			for _, f := range usage.Files {
				fmt.Printf("    - %s\n", f)
			}
		}
		fmt.Println()
	}

	if len(unknownFuncs) > 0 {
		fmt.Printf("--- Unknown/Custom functions (%d) ---\n", len(unknownFuncs))
		fmt.Println("WARNING: These functions are not in the standard FuncMap!")
		for _, usage := range unknownFuncs {
			fmt.Printf("  %s (used %d times)\n", usage.Name, usage.Count)
			for _, f := range usage.Files {
				fmt.Printf("    - %s\n", f)
			}
		}
		fmt.Println()
	}

	// Summary
	if !showUnknown && !showExecOnly && !showInsecure {
		fmt.Println("=== Summary ===")
		fmt.Printf("Helmfile functions: %d\n", len(result.HelmfileFunctions))
		fmt.Printf("Sprig functions: %d\n", len(result.SprigFunctions))
		if len(result.UnknownFunctions) > 0 {
			fmt.Printf("Unknown functions: %d (potential errors!)\n", len(result.UnknownFunctions))
		}
	}
}

func filterFunctions(funcs []*FunctionUsage, names []string) []*FunctionUsage {
	var filtered []*FunctionUsage
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}
	for _, f := range funcs {
		if nameSet[f.Name] {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

func listAllFunctions() {
	r := tmpl.NewFileRenderer(filesystem.DefaultFileSystem(), ".", nil)
	funcMap := r.Context.CreateFuncMap()

	var names []string
	for name := range funcMap {
		names = append(names, name)
	}
	sort.Strings(names)

	helmfileSpecific := map[string]bool{
		"envExec": true, "exec": true, "isFile": true, "isDir": true,
		"readFile": true, "readDir": true, "readDirEntries": true,
		"toYaml": true, "fromYaml": true, "setValueAtPath": true,
		"requiredEnv": true, "get": true, "getOrNil": true,
		"tpl": true, "required": true, "fetchSecretValue": true,
		"expandSecretRefs": true, "sprigGet": true, "include": true,
	}

	fmt.Printf("=== Available Template Functions (%d total) ===\n\n", len(names))

	fmt.Println("--- Helmfile-specific functions ---")
	for _, name := range names {
		if helmfileSpecific[name] {
			fmt.Printf("  %s\n", name)
		}
	}

	fmt.Println("\n--- Sprig functions ---")
	for _, name := range names {
		if !helmfileSpecific[name] {
			fmt.Printf("  %s\n", name)
		}
	}

	fmt.Println("\n--- Potentially insecure functions ---")
	insecure := []string{"exec", "envExec", "readFile", "readDir", "readDirEntries"}
	for _, name := range insecure {
		fmt.Printf("  %s\n", name)
	}
}

// findTemplateFiles finds all helmfile-related template files
func findTemplateFiles(root string) ([]string, error) {
	var files []string
	seen := make(map[string]bool)

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		filename := info.Name()
		matched := false

		// Check for template files
		if strings.HasSuffix(filename, ".gotmpl") || strings.HasSuffix(filename, ".tpl") {
			matched = true
		}

		// Check for helmfile*.yaml files
		if strings.HasPrefix(filename, "helmfile") && strings.HasSuffix(filename, ".yaml") {
			matched = true
		}

		// Check for values files
		if strings.HasPrefix(filename, "values") && (strings.HasSuffix(filename, ".yaml") || strings.HasSuffix(filename, ".yml")) {
			matched = true
		}

		if matched && !seen[path] {
			// Verify it contains template syntax
			content, err := os.ReadFile(path)
			if err == nil && containsTemplateSyntax(string(content)) {
				files = append(files, path)
				seen[path] = true
			}
		}

		return nil
	})

	return files, err
}

// containsTemplateSyntax checks if content has Go template syntax
func containsTemplateSyntax(content string) bool {
	return strings.Contains(content, "{{") && strings.Contains(content, "}}")
}

// extractFunctions extracts function names from template content
func extractFunctions(content string) []string {
	var funcs []string
	seen := make(map[string]bool)

	// Keywords to exclude (Go template keywords)
	keywords := map[string]bool{
		"if": true, "else": true, "end": true, "range": true,
		"with": true, "define": true, "template": true, "block": true,
		"nil": true, "true": true, "false": true, "and": true, "or": true,
		"not": true, "eq": true, "ne": true, "lt": true, "le": true,
		"gt": true, "ge": true, "call": true, "index": true, "slice": true,
		"len": true, "print": true, "printf": true, "println": true,
		"html": true, "js": true, "urlquery": true,
	}

	addFunc := func(name string) {
		if !keywords[name] && !seen[name] && len(name) > 0 {
			if !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "$") {
				funcs = append(funcs, name)
				seen[name] = true
			}
		}
	}

	// Extract only content inside {{ ... }} blocks (with multiline support)
	templateBlockPattern := regexp.MustCompile(`(?s)\{\{-?(.*?)-?\}\}`)
	templateBlocks := templateBlockPattern.FindAllStringSubmatch(content, -1)

	for _, block := range templateBlocks {
		if len(block) < 2 {
			continue
		}
		blockContent := block[1]

		// Skip template comments {{/* ... */}}
		if strings.HasPrefix(strings.TrimSpace(blockContent), "/*") {
			continue
		}

		// Normalize whitespace for easier parsing
		blockContent = strings.ReplaceAll(blockContent, "\n", " ")
		blockContent = strings.ReplaceAll(blockContent, "\t", " ")

		// Pattern for function at the very start of block: {{ funcName
		pattern1 := regexp.MustCompile(`^\s*([a-zA-Z][a-zA-Z0-9_]*)\s`)

		// Pattern for function calls after keywords: if funcName, range funcName, with funcName
		pattern2 := regexp.MustCompile(`(?:if|range|with)\s+([a-zA-Z][a-zA-Z0-9_]*)`)

		// Pattern for assignment: $var := funcName
		pattern3 := regexp.MustCompile(`:=\s*([a-zA-Z][a-zA-Z0-9_]*)`)

		// Pattern for pipeline functions: | funcName
		pattern4 := regexp.MustCompile(`\|\s*([a-zA-Z][a-zA-Z0-9_]*)`)

		// Pattern for function calls with parentheses: (funcName
		pattern5 := regexp.MustCompile(`\(\s*([a-zA-Z][a-zA-Z0-9_]*)`)

		// Pattern for function after another identifier and space: toYaml .Values, get "key" .Map
		pattern6 := regexp.MustCompile(`\s([a-zA-Z][a-zA-Z0-9_]*)\s+["\.\$]`)

		for _, match := range pattern1.FindAllStringSubmatch(blockContent, -1) {
			if len(match) > 1 {
				addFunc(match[1])
			}
		}

		for _, match := range pattern2.FindAllStringSubmatch(blockContent, -1) {
			if len(match) > 1 {
				addFunc(match[1])
			}
		}

		for _, match := range pattern3.FindAllStringSubmatch(blockContent, -1) {
			if len(match) > 1 {
				addFunc(match[1])
			}
		}

		for _, match := range pattern4.FindAllStringSubmatch(blockContent, -1) {
			if len(match) > 1 {
				addFunc(match[1])
			}
		}

		for _, match := range pattern5.FindAllStringSubmatch(blockContent, -1) {
			if len(match) > 1 {
				addFunc(match[1])
			}
		}

		for _, match := range pattern6.FindAllStringSubmatch(blockContent, -1) {
			if len(match) > 1 {
				addFunc(match[1])
			}
		}
	}

	return funcs
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
