package testutil

import "strings"

type ProbeOrderingAlignment struct {
	syntheticCanonicalKey string
	realPageCanonicalKey  string
}

func CanonicalPageKeyForProbeName(name string) string {
	switch {
	case strings.Contains(name, "p95"):
		return "page95"
	case strings.Contains(name, "p109"):
		return "page109"
	default:
		return ""
	}
}

func LargerProbeName(
	page95Name string,
	page95Value float64,
	page109Name string,
	page109Value float64,
) string {
	if page109Value > page95Value {
		return page109Name
	}
	return page95Name
}

func LargerProbeCanonicalPageKey(
	page95Name string,
	page95Value float64,
	page109Name string,
	page109Value float64,
) string {
	return CanonicalPageKeyForProbeName(
		LargerProbeName(page95Name, page95Value, page109Name, page109Value),
	)
}

func SmallerProbeName(
	page95Name string,
	page95Value float64,
	page109Name string,
	page109Value float64,
) string {
	if page109Value < page95Value {
		return page109Name
	}
	return page95Name
}

func SmallerProbeCanonicalPageKey(
	page95Name string,
	page95Value float64,
	page109Name string,
	page109Value float64,
) string {
	return CanonicalPageKeyForProbeName(
		SmallerProbeName(page95Name, page95Value, page109Name, page109Value),
	)
}

func SharedCanonicalPageKey(keys ...string) string {
	if len(keys) == 0 {
		return ""
	}

	shared := keys[0]
	if shared == "" {
		return ""
	}

	for _, key := range keys[1:] {
		if key != shared {
			return ""
		}
	}

	return shared
}

func NewProbeOrderingAlignment(
	syntheticCanonicalKey string,
	realPageCanonicalKey string,
) ProbeOrderingAlignment {
	return ProbeOrderingAlignment{
		syntheticCanonicalKey: syntheticCanonicalKey,
		realPageCanonicalKey:  realPageCanonicalKey,
	}
}

func (a ProbeOrderingAlignment) SharedCanonicalKey() string {
	return SharedCanonicalPageKey(a.syntheticCanonicalKey, a.realPageCanonicalKey)
}
