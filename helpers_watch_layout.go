package enigma

import (
	"context"
	"errors"
)

// WatchLayout is a Go version of @qlik/api's layoutSubscribe.

// LayoutUpdate is a layout sent by WatchLayout, or the error from fetching it.
type LayoutUpdate[T any] struct {
	Layout T
	Err    error
}

// The engine aborts a layout calculation when the object changes during it (LOCERR_GENERIC_ABORTED)
const errorCodeAborted = 15

// WatchLayout fetches a layout with getLayout right away and again each time obj changes, and sends
// them on the returned channel. The channel is closed when ctx is done or obj is closed.
//
// Only up to date layouts are sent: if obj changes again before the layout is read from the channel,
// that layout is dropped and a new one fetched. Errors from getLayout are sent as well, and watching
// continues with the next change. Aborted calculations are not sent, since the change that aborted
// them triggers a new fetch.
//
// getLayout can be any function that reads obj, for example:
//
//	enigma.WatchLayout(ctx, obj.RemoteObject, obj.GetLayoutRaw)
//	enigma.WatchLayout(ctx, sheetList.RemoteObject, sheetList.Items)
//	enigma.WatchLayout(ctx, currentSelection.RemoteObject, currentSelection.Selections)
func WatchLayout[T any](ctx context.Context, obj *RemoteObject, getLayout func(context.Context) (T, error)) <-chan LayoutUpdate[T] {
	updates := make(chan LayoutUpdate[T])
	changed := obj.ChangedChannel()
	go func() {
		defer close(updates)
		defer obj.RemoveChangeChannel(changed)
		for {
			// Changes until now are covered by the fetch below, later ones trigger another fetch
			if !drain(changed) {
				return
			}
			layout, err := getLayout(ctx)
			if ctx.Err() != nil {
				return
			}
			var qixErr Error
			if !errors.As(err, &qixErr) || qixErr.Code() != errorCodeAborted {
				select {
				case updates <- LayoutUpdate[T]{Layout: layout, Err: err}:
				case _, ok := <-changed:
					if !ok {
						return
					}
					// Outdated before anyone read it, fetch again
					continue
				case <-ctx.Done():
					return
				case <-obj.Closed():
					return
				}
			}
			select {
			case _, ok := <-changed:
				if !ok {
					return
				}
			case <-ctx.Done():
				return
			case <-obj.Closed():
				return
			}
		}
	}()
	return updates
}

// drain empties a change channel and returns false if it is closed
func drain(changed chan struct{}) bool {
	for {
		select {
		case _, ok := <-changed:
			if !ok {
				return false
			}
		default:
			return true
		}
	}
}

// WatchLayout sends the app's layout right away and again each time it changes, see the WatchLayout function.
func (doc *Doc) WatchLayout(ctx context.Context) <-chan LayoutUpdate[*NxAppLayout] {
	return WatchLayout(ctx, doc.RemoteObject, doc.GetAppLayout)
}

// WatchLayout sends the object's layout right away and again each time it changes, see the WatchLayout function.
func (obj *GenericObject) WatchLayout(ctx context.Context) <-chan LayoutUpdate[*GenericObjectLayout] {
	return WatchLayout(ctx, obj.RemoteObject, obj.GetLayout)
}

// WatchLayout sends the dimension's layout right away and again each time it changes, see the WatchLayout function.
func (obj *GenericDimension) WatchLayout(ctx context.Context) <-chan LayoutUpdate[*GenericDimensionLayout] {
	return WatchLayout(ctx, obj.RemoteObject, obj.GetLayout)
}

// WatchLayout sends the measure's layout right away and again each time it changes, see the WatchLayout function.
func (obj *GenericMeasure) WatchLayout(ctx context.Context) <-chan LayoutUpdate[*GenericMeasureLayout] {
	return WatchLayout(ctx, obj.RemoteObject, obj.GetLayout)
}

// WatchLayout sends the bookmark's layout right away and again each time it changes, see the WatchLayout function.
func (obj *GenericBookmark) WatchLayout(ctx context.Context) <-chan LayoutUpdate[*GenericBookmarkLayout] {
	return WatchLayout(ctx, obj.RemoteObject, obj.GetLayout)
}

// WatchLayout sends the variable's layout right away and again each time it changes, see the WatchLayout function.
func (obj *GenericVariable) WatchLayout(ctx context.Context) <-chan LayoutUpdate[*GenericVariableLayout] {
	return WatchLayout(ctx, obj.RemoteObject, obj.GetLayout)
}
