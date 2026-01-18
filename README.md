# helmfile-validate

[English](#english) | [Русский](#русский)

---

<a name="english"></a>
# English

## Overview

`helmfile-validate` is a static analysis tool for Helmfile configurations that allows you to scan and validate template function usage in your Helmfile projects. Unlike Helmfile itself, which doesn't provide granular control over function usage, this tool enables you to enforce security policies by selectively allowing or forbidding specific template functions.

## Problem Statement

Helmfile provides powerful template functions (like `exec`, `readFile`, `envExec`, etc.) that can execute arbitrary commands or read files from the filesystem. While these functions are useful, they can pose security risks in certain environments, especially in CI/CD pipelines where you want to restrict what operations can be performed.

**The core issue**: Helmfile doesn't provide a built-in way to:
- Forbid only specific functions (e.g., `exec`) while allowing others
- Enforce a whitelist of allowed functions
- Validate hook usage
- Get a comprehensive report of function usage across all template files

`helmfile-validate` solves this by analyzing your Helmfile configurations before they are executed, allowing you to catch security violations early in your CI/CD pipeline.

## Use Cases

### CI/CD Pipelines

Integrate `helmfile-validate` into your CI/CD pipeline to enforce security policies:

```yaml
# Example GitHub Actions workflow
- name: Validate Helmfile
  run: |
    helmfile-validate -blacklist "exec,envExec" .
    # Pipeline fails if exec or envExec are found
```

### Security Audits

Generate reports of function usage across multiple repositories:

```bash
# Scan and generate JSON report
helmfile-validate -json . > report.json

# Find all insecure function usage
helmfile-validate -insecure .
```

## Features

- ✅ **Function Detection**: Scans all template files (`.gotmpl`, `.yaml`, `.yml`) for function usage
- ✅ **Blacklist Mode**: Forbid specific functions (e.g., `exec`, `readFile`)
- ✅ **Whitelist Mode**: Allow only specific functions
- ✅ **Hook Detection**: Detect and validate Helmfile hooks
- ✅ **JSON Output**: Machine-readable output for integration with other tools
- ✅ **Base File Support**: Scans functions in `bases` files (including `.gotmpl`)
- ✅ **Comment Ignoring**: Ignores functions in commented-out lines
- ✅ **Template Support**: Supports Go templates in regular YAML files (legacy Helmfile behavior)

## Installation

### From Source

```bash
git clone https://github.com/your-org/helmfile-validate.git
cd helmfile-validate
go build -o helmfile-validate .
```

### Binary Release

Download the latest release from the [Releases](https://github.com/your-org/helmfile-validate/releases) page.

## Usage

### Basic Scanning

```bash
# Scan current directory
helmfile-validate .

# Scan specific directory
helmfile-validate /path/to/helmfile

# Output as JSON
helmfile-validate -json .
```

### Filtering Functions

```bash
# Show only exec/envExec usage
helmfile-validate -exec .

# Show only unknown functions
helmfile-validate -unknown .

# Show potentially insecure functions
helmfile-validate -insecure .
```

### Validation Modes

#### Blacklist Mode

Forbid specific functions. The tool exits with code 1 if any blacklisted function is found:

```bash
# Fail if exec or envExec are used
helmfile-validate -blacklist "exec,envExec" .

# Fail if readFile is used
helmfile-validate -blacklist "readFile" .
```

#### Whitelist Mode

Allow only specific functions. The tool exits with code 1 if any function not in the whitelist is found:

```bash
# Only allow toYaml and default functions
helmfile-validate -whitelist "toYaml,default" .
```

#### Hook Validation

Forbid hooks in Helmfile:

```bash
# Fail if any hooks are defined
helmfile-validate -no-hooks .
```

### List Available Functions

```bash
# List all available template functions
helmfile-validate -list
```

## Command Line Options

| Option | Description |
|--------|-------------|
| `-json` | Output results as JSON |
| `-exec` | Show only exec/envExec function usage |
| `-unknown` | Show only unknown functions |
| `-insecure` | Show potentially insecure functions (exec, readFile, etc.) |
| `-list` | List all available template functions and exit |
| `-blacklist <functions>` | Comma-separated list of forbidden functions (exit with error if found) |
| `-whitelist <functions>` | Comma-separated list of allowed functions (exit with error if other functions found) |
| `-no-hooks` | Forbid hooks in helmfile (exit with error if hooks are found) |
| `-no-color` | Disable colored output |

## Exit Codes

- `0`: Success (validation passed if validation mode is used)
- `1`: Validation failed (blacklisted function found, whitelist violation, or hooks found)
- `2`: Invalid arguments (e.g., both `-blacklist` and `-whitelist` specified)

## Examples

### Example 1: CI/CD Pipeline Integration

```yaml
# .github/workflows/validate.yml
name: Validate Helmfile

on: [push, pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Install helmfile-validate
        run: |
          wget https://github.com/your-org/helmfile-validate/releases/latest/download/helmfile-validate-linux-amd64
          chmod +x helmfile-validate-linux-amd64
          sudo mv helmfile-validate-linux-amd64 /usr/local/bin/helmfile-validate
      
      - name: Validate Helmfile
        run: |
          helmfile-validate -blacklist "exec,envExec,readFile" .
```

### Example 2: Security Audit Report

```bash
#!/bin/bash
# audit.sh - Generate security audit report

helmfile-validate -json . > audit-report.json

# Check for insecure functions
if helmfile-validate -insecure . | grep -q "exec\|readFile"; then
  echo "WARNING: Insecure functions detected!"
  exit 1
fi
```

## Output Format

### Text Output

```
Scanning directory: /path/to/helmfile
Found 3 template files: helmfile.yaml, base.gotmpl, values.yaml

=== Used Template Functions ===
Total known functions used: 5

--- Helmfile-specific functions (2) ---
  exec (used 1 times)
    Found in: helmfile.yaml
  toYaml (used 2 times)
    Found in: helmfile.yaml, base.gotmpl

--- Sprig functions (3) ---
  list (used 2 times)
    Found in: helmfile.yaml
  nindent (used 1 times)
    Found in: helmfile.yaml

=== Hooks Found ===
Total hooks found: 1
  Hook in helmfile.yaml (release: app-release)
    Events: presync
    Command: echo "pre-sync"

=== Summary ===
Helmfile functions: 2
Sprig functions: 3
Hooks: 1
```

### JSON Output

```json
{
  "scan": {
    "directory": "/path/to/helmfile",
    "files_scanned": ["helmfile.yaml", "base.gotmpl", "values.yaml"],
    "helmfile_functions": [
      {
        "name": "exec",
        "files": ["helmfile.yaml"],
        "count": 1,
        "is_known": true,
        "category": "helmfile"
      }
    ],
    "sprig_functions": [
      {
        "name": "list",
        "files": ["helmfile.yaml"],
        "count": 2,
        "is_known": true,
        "category": "sprig"
      }
    ],
    "hooks": [
      {
        "file": "helmfile.yaml",
        "release": "app-release",
        "events": ["presync"],
        "command": "echo \"pre-sync\""
      }
    ]
  },
  "validation": {
    "valid": false,
    "violations": [
      {
        "name": "exec",
        "files": ["helmfile.yaml"],
        "count": 1,
        "category": "helmfile"
      }
    ],
    "mode": "blacklist",
    "rules": ["exec"]
  }
}
```

## How It Works

1. **File Discovery**: Recursively scans the directory for Helmfile configuration files (`.yaml`, `.yml`, `.gotmpl`)
2. **Template Parsing**: Uses Helmfile's state loader to parse and understand the configuration structure
3. **Base Resolution**: Follows `bases` references to scan included files
4. **Function Extraction**: Analyzes Go template syntax to extract function calls
5. **Hook Detection**: Parses Helmfile state to detect hooks at both state and release levels
6. **Validation**: Compares found functions against blacklist/whitelist rules
7. **Reporting**: Generates human-readable or JSON output with detailed results

## Limitations

- Functions in commented-out lines are ignored (by design)
- Template rendering is performed in "first-pass" mode to avoid executing `exec`/`readFile` during analysis
- Some complex template expressions may not be fully analyzed

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

<a name="русский"></a>
# Русский

## Обзор

`helmfile-validate` — инструмент статического анализа конфигураций Helmfile, который позволяет сканировать и валидировать использование функций шаблонов в ваших проектах Helmfile. В отличие от самого Helmfile, который не предоставляет детального контроля над использованием функций, этот инструмент позволяет применять политики безопасности, выборочно разрешая или запрещая использование конкретных функций шаблонов.

## Постановка проблемы

Helmfile предоставляет мощные функции шаблонов (такие как `exec`, `readFile`, `envExec` и т.д.), которые могут выполнять произвольные команды или читать файлы из файловой системы. Хотя эти функции полезны, они могут представлять риски безопасности в определенных средах, особенно в CI/CD пайплайнах, где вы хотите ограничить, какие операции могут быть выполнены.

**Основная проблема**: Helmfile не предоставляет встроенного способа:
- Запретить только определенные функции (например, `exec`), разрешая другие
- Применить белый список разрешенных функций
- Валидировать использование хуков
- Получить полный отчет об использовании функций во всех файлах шаблонов

`helmfile-validate` решает эту проблему, анализируя ваши конфигурации Helmfile до их выполнения, позволяя выявлять нарушения безопасности на раннем этапе в вашем CI/CD пайплайне.

## Варианты использования

### CI/CD Пайплайны

Интегрируйте `helmfile-validate` в ваш CI/CD пайплайн для применения политик безопасности:

```yaml
# Пример workflow для GitHub Actions
- name: Validate Helmfile
  run: |
    helmfile-validate -blacklist "exec,envExec" .
    # Пайплайн завершится с ошибкой, если найдены exec или envExec
```

### Аудит безопасности

Генерация отчетов об использовании функций в нескольких репозиториях:

```bash
# Сканирование и генерация JSON отчета
helmfile-validate -json . > report.json

# Найти все использования небезопасных функций
helmfile-validate -insecure .
```

## Возможности

- ✅ **Обнаружение функций**: Сканирует все файлы шаблонов (`.gotmpl`, `.yaml`, `.yml`) на использование функций
- ✅ **Режим черного списка**: Запретить определенные функции (например, `exec`, `readFile`)
- ✅ **Режим белого списка**: Разрешить только определенные функции
- ✅ **Обнаружение хуков**: Обнаруживать и валидировать хуки Helmfile
- ✅ **JSON вывод**: Машиночитаемый вывод для интеграции с другими инструментами
- ✅ **Поддержка базовых файлов**: Сканирует функции в файлах `bases` (включая `.gotmpl`)
- ✅ **Игнорирование комментариев**: Игнорирует функции в закомментированных строках
- ✅ **Поддержка шаблонов**: Поддерживает Go шаблоны в обычных YAML файлах (поведение старых версий Helmfile)

## Установка

### Из исходного кода

```bash
git clone https://github.com/your-org/helmfile-validate.git
cd helmfile-validate
go build -o helmfile-validate .
```

### Бинарный релиз

Скачайте последний релиз со страницы [Releases](https://github.com/your-org/helmfile-validate/releases).

## Использование

### Базовое сканирование

```bash
# Сканировать текущую директорию
helmfile-validate .

# Сканировать конкретную директорию
helmfile-validate /path/to/helmfile

# Вывод в формате JSON
helmfile-validate -json .
```

### Фильтрация функций

```bash
# Показать только использование exec/envExec
helmfile-validate -exec .

# Показать только неизвестные функции
helmfile-validate -unknown .

# Показать потенциально небезопасные функции
helmfile-validate -insecure .
```

### Режимы валидации

#### Режим черного списка

Запретить определенные функции. Инструмент завершится с кодом 1, если найдена любая функция из черного списка:

```bash
# Завершится с ошибкой, если используются exec или envExec
helmfile-validate -blacklist "exec,envExec" .

# Завершится с ошибкой, если используется readFile
helmfile-validate -blacklist "readFile" .
```

#### Режим белого списка

Разрешить только определенные функции. Инструмент завершится с кодом 1, если найдена любая функция, не входящая в белый список:

```bash
# Разрешить только функции toYaml и default
helmfile-validate -whitelist "toYaml,default" .
```

#### Валидация хуков

Запретить хуки в Helmfile:

```bash
# Завершится с ошибкой, если определены какие-либо хуки
helmfile-validate -no-hooks .
```

### Список доступных функций

```bash
# Показать все доступные функции шаблонов
helmfile-validate -list
```

## Параметры командной строки

| Параметр | Описание |
|----------|----------|
| `-json` | Вывод результатов в формате JSON |
| `-exec` | Показать только использование функций exec/envExec |
| `-unknown` | Показать только неизвестные функции |
| `-insecure` | Показать потенциально небезопасные функции (exec, readFile и т.д.) |
| `-list` | Показать все доступные функции шаблонов и выйти |
| `-blacklist <functions>` | Список запрещенных функций через запятую (завершится с ошибкой, если найдены) |
| `-whitelist <functions>` | Список разрешенных функций через запятую (завершится с ошибкой, если найдены другие функции) |
| `-no-hooks` | Запретить хуки в helmfile (завершится с ошибкой, если найдены хуки) |
| `-no-color` | Отключить цветной вывод |

## Коды завершения

- `0`: Успех (валидация пройдена, если используется режим валидации)
- `1`: Валидация не пройдена (найдена функция из черного списка, нарушение белого списка или найдены хуки)
- `2`: Неверные аргументы (например, указаны одновременно `-blacklist` и `-whitelist`)

## Примеры

### Пример 1: Интеграция в CI/CD пайплайн

```yaml
# .github/workflows/validate.yml
name: Validate Helmfile

on: [push, pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Install helmfile-validate
        run: |
          wget https://github.com/your-org/helmfile-validate/releases/latest/download/helmfile-validate-linux-amd64
          chmod +x helmfile-validate-linux-amd64
          sudo mv helmfile-validate-linux-amd64 /usr/local/bin/helmfile-validate
      
      - name: Validate Helmfile
        run: |
          helmfile-validate -blacklist "exec,envExec,readFile" .
```

### Пример 2: Отчет аудита безопасности

```bash
#!/bin/bash
# audit.sh - Генерация отчета аудита безопасности

helmfile-validate -json . > audit-report.json

# Проверить наличие небезопасных функций
if helmfile-validate -insecure . | grep -q "exec\|readFile"; then
  echo "WARNING: Обнаружены небезопасные функции!"
  exit 1
fi
```

## Формат вывода

### Текстовый вывод

```
Scanning directory: /path/to/helmfile
Found 3 template files: helmfile.yaml, base.gotmpl, values.yaml

=== Used Template Functions ===
Total known functions used: 5

--- Helmfile-specific functions (2) ---
  exec (used 1 times)
    Found in: helmfile.yaml
  toYaml (used 2 times)
    Found in: helmfile.yaml, base.gotmpl

--- Sprig functions (3) ---
  list (used 2 times)
    Found in: helmfile.yaml
  nindent (used 1 times)
    Found in: helmfile.yaml

=== Hooks Found ===
Total hooks found: 1
  Hook in helmfile.yaml (release: app-release)
    Events: presync
    Command: echo "pre-sync"

=== Summary ===
Helmfile functions: 2
Sprig functions: 3
Hooks: 1
```

### JSON вывод

```json
{
  "scan": {
    "directory": "/path/to/helmfile",
    "files_scanned": ["helmfile.yaml", "base.gotmpl", "values.yaml"],
    "helmfile_functions": [
      {
        "name": "exec",
        "files": ["helmfile.yaml"],
        "count": 1,
        "is_known": true,
        "category": "helmfile"
      }
    ],
    "sprig_functions": [
      {
        "name": "list",
        "files": ["helmfile.yaml"],
        "count": 2,
        "is_known": true,
        "category": "sprig"
      }
    ],
    "hooks": [
      {
        "file": "helmfile.yaml",
        "release": "app-release",
        "events": ["presync"],
        "command": "echo \"pre-sync\""
      }
    ]
  },
  "validation": {
    "valid": false,
    "violations": [
      {
        "name": "exec",
        "files": ["helmfile.yaml"],
        "count": 1,
        "category": "helmfile"
      }
    ],
    "mode": "blacklist",
    "rules": ["exec"]
  }
}
```

## Как это работает

1. **Обнаружение файлов**: Рекурсивно сканирует директорию на наличие файлов конфигурации Helmfile (`.yaml`, `.yml`, `.gotmpl`)
2. **Парсинг шаблонов**: Использует загрузчик состояния Helmfile для парсинга и понимания структуры конфигурации
3. **Разрешение баз**: Следует ссылкам `bases` для сканирования включенных файлов
4. **Извлечение функций**: Анализирует синтаксис Go шаблонов для извлечения вызовов функций
5. **Обнаружение хуков**: Парсит состояние Helmfile для обнаружения хуков на уровне состояния и релизов
6. **Валидация**: Сравнивает найденные функции с правилами черного/белого списка
7. **Отчетность**: Генерирует человекочитаемый или JSON вывод с подробными результатами

## Ограничения

- Функции в закомментированных строках игнорируются (по дизайну)
- Рендеринг шаблонов выполняется в режиме "first-pass" для избежания выполнения `exec`/`readFile` во время анализа
- Некоторые сложные выражения шаблонов могут быть не полностью проанализированы

## Вклад в проект

Вклад приветствуется! Пожалуйста, не стесняйтесь отправлять Pull Request.

## Лицензия

Этот проект лицензирован под MIT License - см. файл [LICENSE](LICENSE) для деталей.
