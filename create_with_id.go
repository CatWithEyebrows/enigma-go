package enigma

import (
	"context"
	"errors"
)

// ErrIDTaken is returned by the Create...WithID methods when the app already has an object with the requested id.
var ErrIDTaken = errors.New("enigma: object id is already taken")

// When the id in qInfo.qId is already taken, the engine doesn't fail but creates the object with a random
// id. The Create...WithID methods below fail with ErrIDTaken instead, after removing that new object again.
// Without an id in props they behave like the methods they wrap.

// CreateObjectWithID is like CreateObject but returns ErrIDTaken when props.Info.Id is already taken.
func (doc *Doc) CreateObjectWithID(ctx context.Context, props *GenericObjectProperties) (*GenericObject, error) {
	return createWithID(ctx, requestedID(props), func() (*GenericObject, error) { return doc.CreateObject(ctx, props) },
		func(obj *GenericObject) string { return obj.GenericId }, doc.DestroyObject)
}

// CreateSessionObjectWithID is like CreateSessionObject but returns ErrIDTaken when props.Info.Id is already taken.
func (doc *Doc) CreateSessionObjectWithID(ctx context.Context, props *GenericObjectProperties) (*GenericObject, error) {
	return createWithID(ctx, requestedID(props), func() (*GenericObject, error) { return doc.CreateSessionObject(ctx, props) },
		func(obj *GenericObject) string { return obj.GenericId }, doc.DestroySessionObject)
}

// CreateDimensionWithID is like CreateDimension but returns ErrIDTaken when props.Info.Id is already taken.
func (doc *Doc) CreateDimensionWithID(ctx context.Context, props *GenericDimensionProperties) (*GenericDimension, error) {
	return createWithID(ctx, requestedID(props), func() (*GenericDimension, error) { return doc.CreateDimension(ctx, props) },
		func(obj *GenericDimension) string { return obj.GenericId }, doc.DestroyDimension)
}

// CreateMeasureWithID is like CreateMeasure but returns ErrIDTaken when props.Info.Id is already taken.
func (doc *Doc) CreateMeasureWithID(ctx context.Context, props *GenericMeasureProperties) (*GenericMeasure, error) {
	return createWithID(ctx, requestedID(props), func() (*GenericMeasure, error) { return doc.CreateMeasure(ctx, props) },
		func(obj *GenericMeasure) string { return obj.GenericId }, doc.DestroyMeasure)
}

// CreateBookmarkWithID is like CreateBookmark but returns ErrIDTaken when props.Info.Id is already taken.
func (doc *Doc) CreateBookmarkWithID(ctx context.Context, props *GenericBookmarkProperties) (*GenericBookmark, error) {
	return createWithID(ctx, requestedID(props), func() (*GenericBookmark, error) { return doc.CreateBookmark(ctx, props) },
		func(obj *GenericBookmark) string { return obj.GenericId }, doc.DestroyBookmark)
}

// CreateBookmarkExWithID is like CreateBookmarkEx but returns ErrIDTaken when props.Info.Id is already taken.
func (doc *Doc) CreateBookmarkExWithID(ctx context.Context, props *GenericBookmarkProperties, objectIdsToPatch []string) (*GenericBookmark, error) {
	return createWithID(ctx, requestedID(props), func() (*GenericBookmark, error) { return doc.CreateBookmarkEx(ctx, props, objectIdsToPatch) },
		func(obj *GenericBookmark) string { return obj.GenericId }, doc.DestroyBookmark)
}

// CreateChildWithID is like CreateChild but returns ErrIDTaken when props.Info.Id is already taken,
// also by an object that isn't a child of obj. A duplicate child is removed without changing obj's properties.
func (obj *GenericObject) CreateChildWithID(ctx context.Context, props *GenericObjectProperties, propForThis *GenericObjectProperties) (*GenericObject, error) {
	return createWithID(ctx, requestedID(props), func() (*GenericObject, error) { return obj.CreateChild(ctx, props, propForThis) },
		func(child *GenericObject) string { return child.GenericId },
		func(ctx context.Context, id string) (bool, error) { return obj.DestroyChild(ctx, id, nil) })
}

func createWithID[T any](ctx context.Context, requested string, create func() (T, error), createdID func(T) string, destroy func(context.Context, string) (bool, error)) (T, error) {
	var none T
	obj, err := create()
	if err != nil || requested == "" {
		return obj, err
	}
	if id := createdID(obj); id != requested {
		if _, err := destroy(ctx, id); err != nil {
			return none, err
		}
		return none, ErrIDTaken
	}
	return obj, nil
}

// requestedID returns the id in the qInfo of props, or "" when there is none
func requestedID(props any) string {
	var info *NxInfo
	switch p := props.(type) {
	case *GenericObjectProperties:
		if p != nil {
			info = p.Info
		}
	case *GenericDimensionProperties:
		if p != nil {
			info = p.Info
		}
	case *GenericMeasureProperties:
		if p != nil {
			info = p.Info
		}
	case *GenericBookmarkProperties:
		if p != nil {
			info = p.Info
		}
	}
	if info == nil {
		return ""
	}
	return info.Id
}
