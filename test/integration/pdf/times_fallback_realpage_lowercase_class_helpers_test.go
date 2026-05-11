package pdf_test

func realPageLowercaseProbeCasesForResidualClass(class realPageResidualClass) []realPageLowercaseProbeCase {
	var filtered []realPageLowercaseProbeCase
	for _, tc := range realPageLowercaseProbeCases() {
		if tc.dominantResidualClass() == class {
			filtered = append(filtered, tc)
		}
	}
	return filtered
}

func realPageSFRMNonLowerLowercaseProbeCases() []realPageLowercaseProbeCase {
	return realPageLowercaseProbeCasesForResidualClass(realPageResidualClassNonLower)
}

func realPageSFRMLongTailLowercaseProbeCases() []realPageLowercaseProbeCase {
	return realPageLowercaseProbeCasesForResidualClass(realPageResidualClassLongTail)
}

func realPageCMRMixedLowercaseProbeCases() []realPageLowercaseProbeCase {
	return realPageLowercaseProbeCasesForResidualClass(realPageResidualClassMixedLowercase)
}

func realPageBroadLowercaseProbeCases() []realPageLowercaseProbeCase {
	var filtered []realPageLowercaseProbeCase
	for _, tc := range realPageLowercaseProbeCases() {
		if tc.hasLongTailCodeSpec() {
			filtered = append(filtered, tc)
		}
	}
	return filtered
}

func realPageExpandedLowercaseProbeCases() []realPageLowercaseProbeCase {
	var filtered []realPageLowercaseProbeCase
	for _, tc := range realPageLowercaseProbeCases() {
		if tc.hasLongTailCodeSpec() && tc.hasNonLowerCodeSpec() {
			filtered = append(filtered, tc)
		}
	}
	return filtered
}

func realPageBroadLowercaseProbeCasesForResidualClass(class realPageResidualClass) []realPageLowercaseProbeCase {
	var filtered []realPageLowercaseProbeCase
	for _, tc := range realPageBroadLowercaseProbeCases() {
		if tc.dominantResidualClass() == class {
			filtered = append(filtered, tc)
		}
	}
	return filtered
}

func realPageExpandedLowercaseProbeCasesForResidualClass(class realPageResidualClass) []realPageLowercaseProbeCase {
	var filtered []realPageLowercaseProbeCase
	for _, tc := range realPageExpandedLowercaseProbeCases() {
		if tc.dominantResidualClass() == class {
			filtered = append(filtered, tc)
		}
	}
	return filtered
}

func realPageSFRMLongTailBroadLowercaseProbeCases() []realPageLowercaseProbeCase {
	return realPageBroadLowercaseProbeCasesForResidualClass(realPageResidualClassLongTail)
}

func realPageSFRMBroadLowercaseProbeCases() []realPageLowercaseProbeCase {
	return []realPageLowercaseProbeCase{
		realPageSFRMLongTailBroadLowercaseProbeCases()[0],
		realPageSFRMNonLowerBroadLowercaseProbeCases()[0],
	}
}

func realPageSFRMNonLowerExpandedLowercaseProbeCases() []realPageLowercaseProbeCase {
	return realPageExpandedLowercaseProbeCasesForResidualClass(realPageResidualClassNonLower)
}

func realPageCMRMixedBroadLowercaseProbeCases() []realPageLowercaseProbeCase {
	return realPageBroadLowercaseProbeCasesForResidualClass(realPageResidualClassMixedLowercase)
}

func realPageSFRMNonLowerBroadLowercaseProbeCases() []realPageLowercaseProbeCase {
	return realPageBroadLowercaseProbeCasesForResidualClass(realPageResidualClassNonLower)
}

func realPageCMRMixedExpandedLowercaseProbeCases() []realPageLowercaseProbeCase {
	return realPageExpandedLowercaseProbeCasesForResidualClass(realPageResidualClassMixedLowercase)
}

func realPageSFRMLongTailExpandedLowercaseProbeCases() []realPageLowercaseProbeCase {
	return realPageExpandedLowercaseProbeCasesForResidualClass(realPageResidualClassLongTail)
}

func realPageSFRMExpandedLowercaseProbeCases() []realPageLowercaseProbeCase {
	return []realPageLowercaseProbeCase{
		realPageSFRMLongTailExpandedLowercaseProbeCases()[0],
		realPageSFRMNonLowerExpandedLowercaseProbeCases()[0],
	}
}
