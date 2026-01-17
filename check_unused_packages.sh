#!/bin/bash
# Скрипт для проверки использования пакетов из pkg/

echo "=== Проверка использования пакетов из pkg/ ==="
echo ""

# Все пакеты в pkg/
ALL_PKGS=$(go list ./pkg/... 2>/dev/null | grep -E "github.com/seredavin/helmfile-validate/pkg" | sort)

# Пакеты, используемые напрямую из main (включая транзитивные зависимости)
USED_PKGS=$(go list -f '{{.ImportPath}}: {{join .Deps " "}}' . 2>/dev/null | \
  grep -o "github.com/seredavin/helmfile-validate/pkg/[^ ]*" | \
  sort -u)

# Пакеты, используемые напрямую (импорты в main.go)
DIRECT_USED=$(go list -f '{{join .Imports " "}}' . 2>/dev/null | \
  grep -o "github.com/seredavin/helmfile-validate/pkg/[^ ]*" | \
  sort -u)

echo "📊 Статистика:"
echo "   Всего пакетов в pkg/: $(echo "$ALL_PKGS" | wc -l | tr -d ' ')"
echo "   Используемых пакетов (транзитивно): $(echo "$USED_PKGS" | wc -l | tr -d ' ')"
echo "   Используемых напрямую из main: $(echo "$DIRECT_USED" | wc -l | tr -d ' ')"
echo ""

echo "✅ Используемые пакеты (напрямую из main.go):"
echo "$DIRECT_USED" | sed 's/^/   /'
echo ""

echo "📦 Неиспользуемые пакеты (не подключены к main):"
UNUSED=$(comm -23 <(echo "$ALL_PKGS") <(echo "$USED_PKGS"))
if [ -z "$UNUSED" ]; then
  echo "   ✓ Все пакеты используются!"
else
  echo "$UNUSED" | while read pkg; do
    # Проверяем, может это тестовый пакет
    if echo "$pkg" | grep -q "_test\|testdata\|/test\."; then
      echo "   ⚠️  $pkg (тестовый - можно игнорировать)"
    else
      # Проверяем, используется ли внутри других пакетов pkg/helmfile/
      USED_IN_OTHER=$(grep -r "$pkg" pkg/ --include="*.go" 2>/dev/null | grep -v "^$pkg" | head -1)
      if [ -n "$USED_IN_OTHER" ]; then
        echo "   ⚠️  $pkg (используется внутри pkg/helmfile/, но не из main)"
      else
        echo "   ❌ $pkg (полностью не используется)"
      fi
    fi
  done
fi
echo ""
echo "💡 Примечание: пакеты, используемые внутри pkg/helmfile/, могут быть частью"
echo "   скопированного кода helmfile и могут не требоваться для helmfile-validate."
