package pdf

// GetAcroFormTree is a Java-parity alias of FormFieldTree.
func (d *Document) GetAcroFormTree() ([]*FormField, error) {
	return d.FormFieldTree()
}

// GetActionService returns document-scoped action service facade.
func (d *Document) GetActionService() *Document {
	return d
}

// GetAnnotationService returns document-scoped annotation service facade.
func (d *Document) GetAnnotationService() *Document {
	return d
}

// GetBookmarkService returns document-scoped bookmark service facade.
func (d *Document) GetBookmarkService() *Document {
	return d
}

// GetInternalBookmarkService returns document-scoped internal bookmark service facade.
func (d *Document) GetInternalBookmarkService() *Document {
	return d
}

// GetFileAttachmentService returns document-scoped attachment service facade.
func (d *Document) GetFileAttachmentService() *Document {
	return d
}

// GetFormService returns document-scoped form service facade.
func (d *Document) GetFormService() *Document {
	return d
}

// GetOutlineService returns document-scoped outline service facade.
func (d *Document) GetOutlineService() *Document {
	return d
}

// GetSignService returns document-scoped signature service facade.
func (d *Document) GetSignService() *Document {
	return d
}

// GetTextSearchService returns document-scoped text-search service facade.
func (d *Document) GetTextSearchService() *Document {
	return d
}

// GetTextService returns document-scoped text service facade.
func (d *Document) GetTextService() *Document {
	return d
}

// GetUserDataService returns document-scoped user-data service facade.
func (d *Document) GetUserDataService() *Document {
	return d
}

// GetDocInfoService returns document-scoped metadata/info service facade.
func (d *Document) GetDocInfoService() *Document {
	return d
}

// GetPageTransformService returns document-scoped page transform service facade.
func (d *Document) GetPageTransformService() *Document {
	return d
}

// GetMultiplConfigurationService returns document-scoped configuration service facade.
func (d *Document) GetMultiplConfigurationService() *Document {
	return d
}

// GetQuizService returns document-scoped quiz service facade.
func (d *Document) GetQuizService() *Document {
	return d
}
