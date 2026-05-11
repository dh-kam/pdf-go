package pdf

import (
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// GetEncryptFilterSL returns empty string in headless mode.
func (d *Document) GetEncryptFilterSL() string {
	if filter, _, enabled, _ := d.legacyEncryptionState(); enabled && strings.TrimSpace(filter) != "" {
		return filter
	}
	encryptDict := d.encryptDict()
	if encryptDict == nil {
		return ""
	}
	return extractEntityNameOrString(encryptDict.Get(entity.Name("Filter")))
}

// IsEncryptedSL reports whether document is encrypted in native runtime.
func (d *Document) IsEncryptedSL() bool {
	if _, _, enabled, _ := d.legacyEncryptionState(); enabled {
		return true
	}
	if d == nil || d.doc == nil || d.doc.XRef() == nil {
		return false
	}
	if x, ok := d.doc.XRef().(interface{ IsEncrypted() bool }); ok && x.IsEncrypted() {
		return true
	}
	return d.encryptDict() != nil
}

// IsEncryptedAsStandardDRM reports Standard DRM encryption mode.
func (d *Document) IsEncryptedAsStandardDRM() bool {
	return strings.EqualFold(strings.TrimSpace(d.GetEncryptFilterSL()), "Standard")
}

// IsEncryptedAsUnidocsDRM reports Unidocs DRM encryption mode.
func (d *Document) IsEncryptedAsUnidocsDRM() bool {
	filter := strings.ToLower(strings.TrimSpace(d.GetEncryptFilterSL()))
	return strings.Contains(filter, "unidoc") || strings.Contains(filter, "drm")
}

// IsCanSubThreadThumbnailRender reports sub-thread thumbnail render capability.
func (d *Document) IsCanSubThreadThumbnailRender() bool {
	return d.IsOpened()
}

// IsCorruptedAfterSave reports corruption status after save.
func (d *Document) IsCorruptedAfterSave() bool {
	return false
}

// IsEdupdf reports edu-PDF mode state.
func (d *Document) IsEdupdf() bool {
	return false
}

// IsNowHasAvailableFreeTempSpace reports whether temp storage is available.
func (d *Document) IsNowHasAvailableFreeTempSpace() bool {
	return true
}

// IsNowHasHighPriorityWorkingThanRender reports high-priority blocking state.
func (d *Document) IsNowHasHighPriorityWorkingThanRender() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.highPriorityWorkingCount > 0
}

// IsNrdsMoreCachePossible reports NRDS cache expansion capability.
func (d *Document) IsNrdsMoreCachePossible() bool {
	if d == nil {
		return false
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.nrdsCacheLimit <= 0 {
		return false
	}
	return len(d.nrdsTileBitmap) < d.nrdsCacheLimit
}

// IsOpenedFromPackage reports whether current source is packaged container.
func (d *Document) IsOpenedFromPackage() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return strings.EqualFold(d.openFrom, "Package")
}

// IsOwnerPasswordOK reports owner-password authorization status.
func (d *Document) IsOwnerPasswordOK() bool {
	if _, _, enabled, ownerPasswordOK := d.legacyEncryptionState(); enabled {
		return ownerPasswordOK
	}
	if d == nil || d.doc == nil {
		return false
	}
	if d.doc.XRef() == nil {
		return true
	}
	if x, ok := d.doc.XRef().(interface{ IsAuthenticated() bool }); ok {
		return x.IsAuthenticated()
	}
	return true
}

// IsSavedAfterOpen reports whether at least one save operation succeeded.
func (d *Document) IsSavedAfterOpen() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.savedAfterOpen
}

// IsStreamingForOpen reports whether streaming-open mode is active.
func (d *Document) IsStreamingForOpen() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.streamingForOpen
}

// OkToAddNotes reports permission to add notes.
func (d *Document) OkToAddNotes() bool {
	if permissions, ok := d.encryptionPermissions(); ok {
		return permissions.HasPermission(entity.PermAnnotate)
	}
	return true
}

// OkToChange reports permission to modify document.
func (d *Document) OkToChange() bool {
	if permissions, ok := d.encryptionPermissions(); ok {
		return permissions.HasPermission(entity.PermModify)
	}
	return true
}

// OkToCopy reports permission to copy document contents.
func (d *Document) OkToCopy() bool {
	if permissions, ok := d.encryptionPermissions(); ok {
		return permissions.HasPermission(entity.PermCopy)
	}
	return true
}

// OkToPrint reports permission to print document contents.
func (d *Document) OkToPrint() bool {
	if permissions, ok := d.encryptionPermissions(); ok {
		return permissions.HasPermission(entity.PermPrint) ||
			permissions.HasPermission(entity.PermPrintHighRes)
	}
	return true
}

// OkToScreencapture reports permission to capture screen content.
func (d *Document) OkToScreencapture() bool {
	if permissions, ok := d.encryptionPermissions(); ok {
		return permissions.HasPermission(entity.PermCopy) ||
			permissions.HasPermission(entity.PermExtract)
	}
	return true
}

func (d *Document) encryptionPermissions() (entity.PermissionFlags, bool) {
	if _, permissions, enabled, _ := d.legacyEncryptionState(); enabled {
		return permissions, true
	}

	encryptDict := d.encryptDict()
	if encryptDict == nil {
		return 0, false
	}

	pObj := encryptDict.Get(entity.Name("P"))
	if pObj == nil {
		return 0, false
	}

	switch v := pObj.(type) {
	case *entity.Integer:
		return entity.PermissionFlags(uint32(v.Value())), true
	case *entity.Real:
		return entity.PermissionFlags(uint32(int64(v.Value()))), true
	default:
		return 0, false
	}
}

func (d *Document) legacyEncryptionState() (string, entity.PermissionFlags, bool, bool) {
	if d == nil {
		return "", 0, false, false
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.legacyEncryptFilter, d.legacyEncryptPermissions, d.legacyEncryptEnabled, d.legacyOwnerPasswordOK
}

func (d *Document) encryptDict() *entity.Dict {
	if d == nil || d.doc == nil || d.doc.XRef() == nil {
		return nil
	}

	trailerProvider, ok := d.doc.XRef().(interface {
		GetTrailer() (*entity.Dict, error)
	})
	if !ok {
		return nil
	}

	trailer, err := trailerProvider.GetTrailer()
	if err != nil || trailer == nil {
		return nil
	}

	encryptObj := trailer.Get(entity.Name("Encrypt"))
	if encryptObj == nil {
		return nil
	}

	switch v := encryptObj.(type) {
	case *entity.Dict:
		return v
	case entity.Ref:
		resolved, fetchErr := d.doc.XRef().Fetch(v)
		if fetchErr != nil {
			return nil
		}
		dict, ok := resolved.(*entity.Dict)
		if !ok {
			return nil
		}
		return dict
	default:
		return nil
	}
}
