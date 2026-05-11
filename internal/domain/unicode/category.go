package unicode

import (
	"sync"
	"unicode"
)

// CharUnicodeCategory represents the category of a Unicode character.
type CharUnicodeCategory struct {
	// IsWhitespace indicates if the character is whitespace
	IsWhitespace bool

	// IsZeroWidthDiacritic indicates if the character is a nonspacing mark (diacritic)
	// Corresponds to Unicode category Mn (Mark, Nonspacing)
	IsZeroWidthDiacritic bool

	// IsInvisibleFormatMark indicates if the character is an invisible format mark
	// Corresponds to Unicode category Cf (Other, Format)
	IsInvisibleFormatMark bool
}

var (
	// categoryCache stores computed categories for performance
	categoryCache = make(map[rune]CharUnicodeCategory)

	// cacheMutex protects concurrent access to categoryCache
	cacheMutex sync.RWMutex
)

// GetCharUnicodeCategory returns the Unicode category information for a character.
// Results are cached for performance.
func GetCharUnicodeCategory(char rune) CharUnicodeCategory {
	// Try to get from cache first (read lock)
	cacheMutex.RLock()
	if category, ok := categoryCache[char]; ok {
		cacheMutex.RUnlock()
		return category
	}
	cacheMutex.RUnlock()

	// Compute category
	category := CharUnicodeCategory{
		IsWhitespace:          unicode.IsSpace(char),
		IsZeroWidthDiacritic:  unicode.Is(unicode.Mn, char), // Mark, Nonspacing
		IsInvisibleFormatMark: unicode.Is(unicode.Cf, char), // Other, Format
	}

	// Store in cache (write lock)
	cacheMutex.Lock()
	categoryCache[char] = category
	cacheMutex.Unlock()

	return category
}

// ClearUnicodeCaches clears all cached Unicode category data.
// This is useful for testing or memory management.
func ClearUnicodeCaches() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	// Clear the map
	categoryCache = make(map[rune]CharUnicodeCategory)
}

// GetCacheSize returns the current size of the category cache.
// This is primarily useful for testing and diagnostics.
func GetCacheSize() int {
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()

	return len(categoryCache)
}
