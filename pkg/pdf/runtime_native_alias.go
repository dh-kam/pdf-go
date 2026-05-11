package pdf

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/dh-kam/pdf-go/internal/domain/entity"
)

// AbortMultithreadRendering aborts multithread rendering in native runtime.
func (d *Document) AbortMultithreadRendering(_ int) int {
	d.clearRenderingState(true)
	return 0
}

// AbortRenderingForThumbnailSL aborts thumbnail rendering in native runtime.
func (d *Document) AbortRenderingForThumbnailSL() int {
	d.clearRenderingState(false)
	return 0
}

// AbortRenderingSL aborts rendering in native runtime.
func (d *Document) AbortRenderingSL() int {
	d.clearRenderingState(true)
	return 0
}

// DescribeRemainNativeCall returns remaining native call description.
func (d *Document) DescribeRemainNativeCall() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return fmt.Sprintf(
		"render=%d thumbnail=%d high_priority=%d",
		d.nowRenderingCount,
		d.nowThumbnailRenderCount,
		d.highPriorityWorkingCount,
	)
}

// DirectReloadSubLibrariesForCorruptedState resets transient runtime caches/counters.
func (d *Document) DirectReloadSubLibrariesForCorruptedState() {
	d.clearRenderingState(true)

	d.mu.Lock()
	defer d.mu.Unlock()
	d.highPriorityWorkingCount = 0
	d.actionContentReplaceList = nil
	d.nrdsTileData = make(map[string][]byte)
	d.nrdsTileBitmap = make(map[string]interface{})
}

// DisposeActionContentReplaceList clears action-content replacement entries.
func (d *Document) DisposeActionContentReplaceList() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.actionContentReplaceList = nil
}

// FirePackagedPDFDocumentBeforeEntryOpen dispatches packaged-document open hook to listener.
func (d *Document) FirePackagedPDFDocumentBeforeEntryOpen(entry string) {
	d.mu.RLock()
	listener := d.packagedDocListener
	d.mu.RUnlock()
	if listener == nil {
		return
	}

	switch fn := listener.(type) {
	case func(string):
		fn(entry)
		return
	case func():
		fn()
		return
	case interface {
		FirePackagedPDFDocumentBeforeEntryOpen(string)
	}:
		fn.FirePackagedPDFDocumentBeforeEntryOpen(entry)
		return
	}

	if callLegacyPackagedListener(listener, "FirePackagedPDFDocumentBeforeEntryOpen", entry) {
		return
	}
	if callLegacyPackagedListener(listener, "BeforeEntryOpen", entry) {
		return
	}
	_ = callLegacyPackagedListener(listener, "OnBeforeEntryOpen", entry)
}

// GetActivationPrivatePieceInfo returns activation private piece info.
func (d *Document) GetActivationPrivatePieceInfo() string {
	return ""
}

// GetDRMMethod returns DRM method code.
func (d *Document) GetDRMMethod() int {
	if !d.IsEncryptedSL() {
		return 0
	}
	filter := strings.ToLower(strings.TrimSpace(d.GetEncryptFilterSL()))
	switch {
	case strings.Contains(filter, "unidoc"), strings.Contains(filter, "drm"):
		return 2
	default:
		return 1
	}
}

// GetDocId returns one stable document id for current content.
//
//nolint:revive // Java parity method name keeps Id casing.
func (d *Document) GetDocId() string {
	if key := d.GetDocKeys1(); strings.TrimSpace(key) != "" {
		return key
	}
	return d.GetMutableUid()
}

// GetDocKeys1 returns document key #1 when available.
func (d *Document) GetDocKeys1() string {
	return d.lookupLegacyTrailerID(0)
}

// GetDocKeys2 returns document key #2 when available.
func (d *Document) GetDocKeys2() string {
	return d.lookupLegacyTrailerID(1)
}

// GetMemoryLackCount returns memory-pressure counter.
func (d *Document) GetMemoryLackCount() int {
	return 0
}

// GetMutableUid returns one best-effort mutable uid for current state.
//
//nolint:revive // Java parity method name keeps Uid casing.
func (d *Document) GetMutableUid() string {
	if path := strings.TrimSpace(d.GetFilePath()); path != "" {
		return hashStringSHA1(path)
	}

	raw, err := d.rawPDFData()
	if err == nil && len(raw) > 0 {
		return hashBytesSHA1(raw)
	}
	return ""
}

// LockDocStream returns a compatibility stream handle for the current raw PDF bytes.
func (d *Document) LockDocStream() int {
	raw, err := d.rawPDFData()
	if err != nil {
		return 0
	}
	return d.allocLegacyStreamHandle(raw)
}

// LookupDocInfoSL returns one document-info string value by key.
func (d *Document) LookupDocInfoSL(key string) string {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" || d == nil || d.doc == nil {
		return ""
	}

	info := d.doc.Info()
	if info == nil {
		return ""
	}

	value := info.Get(entity.Name(trimmed))
	if value == nil {
		return ""
	}

	switch v := value.(type) {
	case *entity.String:
		return v.Value()
	case entity.Name:
		return string(v)
	case *entity.Integer:
		return strconv.FormatInt(v.Value(), 10)
	case *entity.Real:
		return strconv.FormatFloat(v.Value(), 'f', -1, 64)
	case *entity.Boolean:
		return strconv.FormatBool(v.Value())
	default:
		return ""
	}
}

func hashStringSHA1(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func hashBytesSHA1(value []byte) string {
	sum := sha1.Sum(value)
	return hex.EncodeToString(sum[:])
}

func (d *Document) clearRenderingState(includeNormal bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if includeNormal {
		d.nowRenderingCount = 0
	}
	d.nowThumbnailRenderCount = 0
}

func callLegacyPackagedListener(listener interface{}, methodName string, entry string) bool {
	if listener == nil || strings.TrimSpace(methodName) == "" {
		return false
	}

	method := reflect.ValueOf(listener).MethodByName(methodName)
	if !method.IsValid() {
		return false
	}

	methodType := method.Type()
	switch methodType.NumIn() {
	case 0:
		method.Call(nil)
		return true
	case 1:
		entryValue := reflect.ValueOf(entry)
		parameterType := methodType.In(0)
		if entryValue.Type().AssignableTo(parameterType) {
			method.Call([]reflect.Value{entryValue})
			return true
		}
		if entryValue.Type().ConvertibleTo(parameterType) {
			method.Call([]reflect.Value{entryValue.Convert(parameterType)})
			return true
		}
	}

	return false
}
