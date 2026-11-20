package locales

import (
	"strings"

	"github.com/egoist/mygo/i18n"
)

// IsChinese reports whether the active app locale is Chinese.
func IsChinese() bool {
	return strings.HasPrefix(strings.ToLower(i18n.Locale()), "zh")
}
