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
	"github.com/helmfile/vals"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/seredavin/helmfile-validate/pkg/helmfile/environment"
	"github.com/seredavin/helmfile-validate/pkg/helmfile/filesystem"
	"github.com/seredavin/helmfile-validate/pkg/helmfile/helmexec"
	"github.com/seredavin/helmfile-validate/pkg/helmfile/remote"
	"github.com/seredavin/helmfile-validate/pkg/helmfile/state"
	"github.com/seredavin/helmfile-validate/pkg/helmfile/tmpl"
	"github.com/seredavin/helmfile-validate/pkg/helmfile/tracking"
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
	if err := encoder.Encode(output); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON output: %v\n", err)
		os.Exit(1)
	}
}

func outputValidation(validation *ValidationResult) {
	red := color.New(color.FgRed, color.Bold)
	yellow := color.New(color.FgYellow)
	cyan := color.New(color.FgCyan)

	fmt.Println()
	if validation.Valid {
		green := color.New(color.FgGreen, color.Bold)
		_, _ = green.Println("=== Validation PASSED ===")
		fmt.Printf("Mode: %s\n", validation.Mode)
		fmt.Printf("Rules: %s\n", strings.Join(validation.Rules, ", "))
		return
	}

	_, _ = red.Println("=== Validation FAILED ===")
	fmt.Println()

	if validation.Mode == "blacklist" {
		_, _ = red.Printf("BLACKLISTED functions found!\n")
		fmt.Printf("Forbidden functions: %s\n\n", strings.Join(validation.Rules, ", "))
	} else {
		_, _ = red.Printf("Functions NOT in WHITELIST found!\n")
		fmt.Printf("Allowed functions: %s\n\n", strings.Join(validation.Rules, ", "))
	}

	_, _ = red.Printf("Violations (%d):\n", len(validation.Violations))
	fmt.Println()

	for _, v := range validation.Violations {
		_, _ = yellow.Printf("  ✗ %s", v.Name)
		fmt.Printf(" (category: %s, used %d times)\n", v.Category, v.Count)
		fmt.Println("    Found in:")
		for _, file := range v.Files {
			_, _ = cyan.Printf("      - %s\n", file)
		}
		fmt.Println()
	}

	_, _ = red.Println("Please remove or replace the forbidden functions to pass validation.")
}

// resolvePath resolves a file path relative to baseDir, handling both absolute and relative paths
func resolvePath(baseDir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(baseDir, path))
}

// findHelmfileYaml finds the main helmfile.yaml file in the directory
func findHelmfileYaml(dir string) (string, error) {
	// Try helmfile.yaml.gotmpl first, then helmfile.yaml, then helmfile.yml
	candidates := []string{
		filepath.Join(dir, "helmfile.yaml.gotmpl"),
		filepath.Join(dir, "helmfile.yaml"),
		filepath.Join(dir, "helmfile.yml"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no helmfile.yaml, helmfile.yml, or helmfile.yaml.gotmpl found in %s", dir)
}

// scanDirectoryUsingStateCreator uses helmfile's StateCreator to discover all template files
func scanDirectoryUsingStateCreator(absPath string) *ScanResult {
	// Get available FuncMap for function categorization
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

	result := &ScanResult{
		Directory:    absPath,
		FilesScanned: []string{},
	}

	// Find main helmfile.yaml
	mainFile, err := findHelmfileYaml(absPath)
	if err != nil {
		return result
	}

	// Create tracking filesystem
	baseFs := filesystem.DefaultFileSystem()
	trackingFs := tracking.NewTrackingFileSystem(baseFs)

	// Create logger
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zapcore.ErrorLevel) // Suppress most logs
	logger, err := config.Build()
	if err != nil {
		// If logger creation fails, continue without logging
		// This is non-critical for file discovery
		return result
	}
	defer func() {
		_ = logger.Sync() // Best effort sync on exit
	}()
	sugarLogger := logger.Sugar()

	// Create vals runtime (dummy, we don't need it for file discovery)
	valsRuntime := &vals.Runtime{}

	// Create dummy getHelm function
	getHelm := func(*state.HelmState) (helmexec.Interface, error) {
		return nil, fmt.Errorf("not implemented")
	}

	// Create remote
	rem := remote.NewRemote(sugarLogger, "", trackingFs.FileSystem)

	// Create StateCreator
	creator := state.NewCreator(sugarLogger, trackingFs.FileSystem, valsRuntime, getHelm, "", "", rem, false, "")

	// Create LoadFile function for StateCreator
	creator.LoadFile = func(inheritedEnv, overrodeEnv *environment.Environment, baseDir, file string, evaluateBases bool) (*state.HelmState, error) {
		fileBytes, err := trackingFs.ReadFile(file)
		if err != nil {
			return nil, err
		}

		// Render template if it's a .gotmpl file
		if strings.HasSuffix(file, ".gotmpl") {
			// Get merged values for template rendering
			var templateData map[string]any
			if inheritedEnv != nil {
				templateData = inheritedEnv.Values
			} else {
				templateData = make(map[string]any)
			}
			if overrodeEnv != nil {
				// Merge override values
				for k, v := range overrodeEnv.Values {
					templateData[k] = v
				}
			}

			// Create renderer and render template
			renderer := tmpl.NewFileRenderer(trackingFs.FileSystem, baseDir, templateData)
			renderedBytes, err := renderer.RenderToBytes(file)
			if err != nil {
				// If rendering fails, return error but still track the file
				return nil, fmt.Errorf("failed to render template %s: %w", file, err)
			}
			fileBytes = renderedBytes
		}

		return creator.ParseAndLoad(fileBytes, baseDir, file, "", false, evaluateBases, inheritedEnv, overrodeEnv)
	}

	// Load the main helmfile state
	// We don't need environment values for file discovery, so use empty environment
	emptyEnv := environment.New("")
	baseDir := filepath.Dir(mainFile)

	// Render template if main file is .gotmpl
	fileBytes, err := trackingFs.ReadFile(mainFile)
	if err != nil {
		return result
	}

	// Parse the file first to get bases before full load
	// This allows us to track base files explicitly
	var parsedState *state.HelmState
	if strings.HasSuffix(mainFile, ".gotmpl") {
		renderer := tmpl.NewFileRenderer(trackingFs.FileSystem, baseDir, emptyEnv.Values)
		renderedBytes, err := renderer.RenderToBytes(mainFile)
		if err != nil {
			// If rendering fails, still try to parse
		} else {
			fileBytes = renderedBytes
		}
	}

	// Parse to get bases list before full load
	parsedState, err = creator.Parse(fileBytes, baseDir, mainFile)
	if err == nil && parsedState != nil {
		// Track base files explicitly before they're loaded
		for _, basePath := range parsedState.Bases {
			resolvedBasePath := resolvePath(baseDir, basePath)
			// Read the base file to ensure it's tracked
			_, _ = trackingFs.ReadFile(resolvedBasePath)
			// If it's a .gotmpl file, also try to render it to track template usage
			if strings.HasSuffix(resolvedBasePath, ".gotmpl") {
				templateData := emptyEnv.Values
				if templateData == nil {
					templateData = make(map[string]any)
				}
				renderer := tmpl.NewFileRenderer(trackingFs.FileSystem, baseDir, templateData)
				_, _ = renderer.RenderToBytes(resolvedBasePath)
			}
		}
	}

	state, err := creator.ParseAndLoad(fileBytes, baseDir, mainFile, "", true, true, emptyEnv, nil)
	if err != nil {
		// Ignore errors related to helm execution, we just need to discover files
		// But log other errors for debugging
		if !strings.Contains(err.Error(), "not implemented") && !strings.Contains(err.Error(), "helm") {
			fmt.Fprintf(os.Stderr, "Warning: error loading helmfile state: %v\n", err)
		}
	} else if state != nil {
		// Explicitly track base files that were loaded
		// Bases are loaded via LoadFile which should track them, but let's also explicitly read them
		// to ensure they're tracked even if LoadFile doesn't fully process them
		for _, basePath := range state.Bases {
			resolvedBasePath := resolvePath(baseDir, basePath)
			// Read the base file to ensure it's tracked
			_, _ = trackingFs.ReadFile(resolvedBasePath)
			// If it's a .gotmpl file, also try to render it to track template usage
			if strings.HasSuffix(resolvedBasePath, ".gotmpl") {
				templateData := state.RenderedValues
				if templateData == nil {
					templateData = make(map[string]any)
				}
				renderer := tmpl.NewFileRenderer(trackingFs.FileSystem, baseDir, templateData)
				_, _ = renderer.RenderToBytes(resolvedBasePath)
			}
		}
		// Process helmfiles field to load nested helmfile files
		// For template helmfiles (like build.gotmpl with readDir), we need to manually
		// extract the pattern and find files, since full rendering requires values that may not be available
		for _, hf := range state.Helmfiles {
			helmfilePath := resolvePath(baseDir, hf.Path)

			// If it's a template helmfile, try to read it and check if it uses readDir
			if strings.HasSuffix(helmfilePath, ".gotmpl") {
				content, err := trackingFs.ReadFile(helmfilePath)
				if err == nil {
					contentStr := string(content)
					// Check if it contains readDir pattern - common pattern: readDir "directory"
					if strings.Contains(contentStr, "readDir") {
						// Try to extract directory pattern and manually find files
						// Common pattern: {{ range $file := readDir "releases" }}
						readDirPattern := regexp.MustCompile(`readDir\s+"([^"]+)"`)
						matches := readDirPattern.FindAllStringSubmatch(contentStr, -1)
						for _, match := range matches {
							if len(match) > 1 {
								dirPath := filepath.Join(baseDir, match[1])
								// Try to read directory contents and track files
								entries, err := os.ReadDir(dirPath)
								if err == nil {
									for _, entry := range entries {
										if !entry.IsDir() {
											filePath := filepath.Join(dirPath, entry.Name())
											_, _ = trackingFs.ReadFile(filePath)
										}
									}
								}
							}
						}
					}
					// Also try to render it (may fail but will track the file)
					templateData := state.RenderedValues
					if templateData == nil {
						templateData = make(map[string]any)
					}
					renderer := tmpl.NewFileRenderer(trackingFs.FileSystem, baseDir, templateData)
					_, _ = renderer.RenderToBytes(helmfilePath)
				}
			}
		}

		// Now expand and load all helmfiles
		expandedHelmfiles, err := state.ExpandedHelmfiles()
		if err == nil {
			for _, hf := range expandedHelmfiles {
				helmfilePath := resolvePath(baseDir, hf.Path)
				// Load nested helmfile to track its files
				_, err := creator.LoadFile(emptyEnv, nil, baseDir, helmfilePath, true)
				if err != nil {
					// But still try to read the file to track it
					_, _ = trackingFs.ReadFile(helmfilePath)
				}

				// Also load values files from helmfiles
				for _, val := range hf.Environment.OverrideValues {
					if valStr, ok := val.(string); ok && valStr != "" {
						valPath := resolvePath(baseDir, valStr)
						_, _ = trackingFs.ReadFile(valPath)
					}
				}
			}
		}

		// Track values files from releases (including files in parent directories)
		for _, release := range state.Releases {
			for _, val := range release.Values {
				if valStr, ok := val.(string); ok && valStr != "" {
					// Resolve path relative to baseDir (can be relative like ../values.yaml)
					valPath := resolvePath(baseDir, valStr)
					// Track the file (this will read it if it exists and contains templates)
					_, _ = trackingFs.ReadFile(valPath)
				}
			}

			// Also track secrets files
			for _, secret := range release.Secrets {
				if secretStr, ok := secret.(string); ok && secretStr != "" {
					secretPath := resolvePath(baseDir, secretStr)
					_, _ = trackingFs.ReadFile(secretPath)
				}
			}
		}

		// Extract hooks from state
		// Hooks can be at the state level (state.Hooks) or at the release level (release.Hooks)
		relMainFile, err := filepath.Rel(absPath, mainFile)
		if err != nil {
			relMainFile = mainFile
		}

		// Extract state-level hooks
		for _, hook := range state.Hooks {
			hookUsage := &HookUsage{
				File:    relMainFile,
				Events:  hook.Events,
				Command: hook.Command,
			}
			result.Hooks = append(result.Hooks, hookUsage)
		}

		// Extract release-level hooks
		for _, release := range state.Releases {
			for _, hook := range release.Hooks {
				hookUsage := &HookUsage{
					File:    relMainFile,
					Release: release.Name,
					Events:  hook.Events,
					Command: hook.Command,
				}
				result.Hooks = append(result.Hooks, hookUsage)
			}
		}
	}

	// Get all files that were read/globbed
	allFiles := trackingFs.GetAllFiles()

	// Also find helper templates (_*.tpl) in the directory
	helperTemplates, err := trackingFs.Glob(filepath.Join(absPath, "_*.tpl"))
	if err == nil {
		allFiles = append(allFiles, helperTemplates...)
	}

	if len(allFiles) == 0 {
		return result
	}

	// Scan files for function usage
	usageMap := make(map[string]*FunctionUsage)
	seenFiles := make(map[string]bool)

	for _, file := range allFiles {
		// Skip non-template files (only process .yaml, .yml, .gotmpl, .tpl files)
		ext := filepath.Ext(file)
		if ext != ".yaml" && ext != ".yml" && ext != ".gotmpl" && ext != ".tpl" {
			continue
		}

		// Skip if we've already processed this file
		if seenFiles[file] {
			continue
		}
		seenFiles[file] = true

		// Check if file exists and has template syntax
		content, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		// Only process files with template syntax
		if !containsTemplateSyntax(string(content)) {
			continue
		}

		relPath, err := filepath.Rel(absPath, file)
		if err != nil {
			// If relative path calculation fails, use absolute path as fallback
			relPath = file
		}
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

func scanDirectory(absPath string) *ScanResult {
	// Use StateCreator-based approach instead of file walking
	return scanDirectoryUsingStateCreator(absPath)
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
