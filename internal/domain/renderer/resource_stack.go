package renderer

import "github.com/dh-kam/pdf-go/internal/domain/entity"

func (e *Evaluator) resourceFramesForLookup() []*entity.Dict {
	if len(e.resourceStack) > 0 {
		return e.resourceStack
	}
	if e.resources != nil {
		return []*entity.Dict{e.resources}
	}
	return nil
}

func (e *Evaluator) hasResourceFrames() bool {
	return len(e.resourceFramesForLookup()) > 0
}

func (e *Evaluator) replaceResourceStack(frames []*entity.Dict) {
	e.resourceStack = append([]*entity.Dict(nil), frames...)
	if len(e.resourceStack) == 0 {
		e.resources = nil
		return
	}
	e.resources = e.resourceStack[0]
}

func (e *Evaluator) pushResources(resources *entity.Dict) func() {
	previousStack := append([]*entity.Dict(nil), e.resourceFramesForLookup()...)
	previousResources := e.resources

	nextStack := make([]*entity.Dict, 0, len(previousStack)+1)
	nextStack = append(nextStack, resources)
	nextStack = append(nextStack, previousStack...)
	e.resourceStack = nextStack
	e.resources = resources

	return func() {
		e.resourceStack = previousStack
		e.resources = previousResources
	}
}

func (e *Evaluator) currentResourceStack() []*entity.Dict {
	return append([]*entity.Dict(nil), e.resourceFramesForLookup()...)
}

func (e *Evaluator) useResourceStack(frames []*entity.Dict) func() {
	previousStack := append([]*entity.Dict(nil), e.resourceStack...)
	previousResources := e.resources
	e.replaceResourceStack(frames)
	return func() {
		e.resourceStack = previousStack
		e.resources = previousResources
	}
}

func (e *Evaluator) getResourceEntryWithFrames(category entity.Name, name entity.Name) (entity.Object, []*entity.Dict) {
	frames := e.resourceFramesForLookup()
	if len(frames) == 0 {
		return nil, nil
	}

	for i, resources := range frames {
		if resourceObj := e.lookupResourceCategoryEntry(resources, category, name); resourceObj != nil {
			return resourceObj, append([]*entity.Dict(nil), frames[i:]...)
		}
	}
	for i, resources := range frames {
		if resources == nil {
			continue
		}
		if resourceObj := e.resolveResourceEntryObject(resources.Get(name), 0); resourceObj != nil {
			return resourceObj, append([]*entity.Dict(nil), frames[i:]...)
		}
	}
	return nil, nil
}

func (e *Evaluator) lookupResourceCategoryEntry(resources *entity.Dict, category entity.Name, name entity.Name) entity.Object {
	if resources == nil {
		return nil
	}
	categoryObj := e.resolveResourceEntryObject(resources.Get(category), 0)
	if categoryStream, ok := categoryObj.(*entity.Stream); ok {
		categoryObj = categoryStream.Dict()
	}
	if categoryDict, ok := categoryObj.(*entity.Dict); ok {
		return e.resolveResourceEntryObject(categoryDict.Get(name), 0)
	}
	return nil
}
