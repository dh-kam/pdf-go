package renderer

import "strings"

func stripSubsetPrefix(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "/")
	if plus := strings.Index(name, "+"); plus >= 0 && plus+1 < len(name) {
		return name[plus+1:]
	}
	return name
}

func normalizeBaseFontName(name string) string {
	name = stripSubsetPrefix(strings.TrimSpace(name))

	switch name {
	case "NimbusRomNo9L-Regu":
		return "Times-Roman"
	case "NimbusRomNo9L-Medi":
		return "Times-Bold"
	case "NimbusRomNo9L-ReguItal", "NimbusRomNo9L-Regu-Slant_167":
		return "Times-Italic"
	case "NimbusRomNo9L-MediItal":
		return "Times-BoldItalic"
	case "Helvetica", "Arial", "YuMincho-Regular":
		return "Helvetica"
	case "NimbusSanL-Bold":
		return "Helvetica-Bold"
	case "Helvetica-Bold", "Arial,Bold", "Calibri-Bold":
		return "Helvetica-Bold"
	case "Helvetica-Oblique", "Arial,Italic", "Calibri-Italic":
		return "Helvetica-Oblique"
	case "Helvetica-BoldOblique", "Arial,BoldItalic", "Calibri-BoldItalic":
		return "Helvetica-BoldOblique"
	}

	switch {
	case strings.HasPrefix(name, "CMR"):
		return "Times-Roman"
	case strings.HasPrefix(name, "CMMI"):
		return "Times-Italic"
	case strings.HasPrefix(name, "CMSY"):
		return "Symbol"
	case strings.HasPrefix(name, "CMTT"):
		return "Courier"
	case strings.HasPrefix(name, "SFTT"):
		return "Courier"
	case strings.HasPrefix(name, "SFSX"):
		return "Helvetica"
	case strings.HasPrefix(name, "SFTI"):
		// LaTeX cm-super italic series (e.g. SFTI1200, SFTI1440).
		return "Times-Italic"
	case strings.HasPrefix(name, "SFRM"):
		// LaTeX cm-super roman medium (e.g. SFRM0900, SFRM1200).
		return "Times-Roman"
	case strings.HasPrefix(name, "SFBX"):
		// LaTeX cm-super bold extended roman (cmbx).
		return "Times-Bold"
	case strings.HasPrefix(name, "Calibri-Bold"):
		return "Helvetica-Bold"
	case strings.HasPrefix(name, "Calibri"):
		return "Helvetica"
	default:
		return name
	}
}
