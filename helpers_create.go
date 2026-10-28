package enigma

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
)

// The creation helpers below are ported from @qlik/api's createSheet, createStory and createMasterObject,
// and the story and slide mixins. They create the objects with the properties the Qlik Sense client expects.

// createAppObject creates an object from props, or with ErrIDTaken when id is set and taken
func (doc *Doc) createAppObject(ctx context.Context, id string, props map[string]any) (*GenericObject, error) {
	if id != "" {
		props["qInfo"].(map[string]any)["qId"] = id
	}
	return createWithID(ctx, id, func() (*GenericObject, error) { return doc.CreateObjectRaw(ctx, props) },
		func(obj *GenericObject) string { return obj.GenericId }, doc.DestroyObject)
}

func thumbnailProps(url string) map[string]any {
	if url == "" {
		return map[string]any{"qStaticContentUrlDef": map[string]any{}}
	}
	return map[string]any{"qStaticContentUrlDef": map[string]any{"qUrl": url}}
}

func rankOrLast(rank *float64) float64 {
	if rank == nil {
		return -1
	}
	return *rank
}

// Sheets

// SheetOptions are the properties of a new sheet. Zero values get the defaults.
type SheetOptions struct {
	// The sheet's qInfo.qId. Empty lets the engine choose one.
	ID          string
	Title       string
	Description string
	// URL of the thumbnail image
	Thumbnail string
	// Position among the sheets. nil puts the sheet first, like a rank of -1.
	Rank *float64
	// Grid size, 24 columns and 12 rows by default
	Columns int
	Rows    int
	// Number of rows for the grid resolution
	CustomRowBase int
	// Number of grid cells: small, medium or large
	GridResolution string
	LayoutOptions  json.RawMessage
	// More properties, set after the ones above
	Properties map[string]any
}

// CreateSheet creates a sheet with the properties the Qlik Sense client expects.
// It returns ErrIDTaken when opts.ID is set and already taken.
func (doc *Doc) CreateSheet(ctx context.Context, opts SheetOptions) (*GenericObject, error) {
	props := map[string]any{
		"qInfo":         map[string]any{"qType": "sheet"},
		"qMetaDef":      map[string]any{"title": opts.Title, "description": opts.Description},
		"rank":          rankOrLast(opts.Rank),
		"thumbnail":     thumbnailProps(opts.Thumbnail),
		"columns":       cmp.Or(opts.Columns, 24),
		"rows":          cmp.Or(opts.Rows, 12),
		"cells":         []any{},
		"qChildListDef": map[string]any{"qData": map[string]any{"title": "/title"}},
	}
	if opts.CustomRowBase != 0 {
		props["customRowBase"] = opts.CustomRowBase
	}
	if opts.GridResolution != "" {
		props["gridResolution"] = opts.GridResolution
	}
	if len(opts.LayoutOptions) > 0 {
		props["layoutOptions"] = opts.LayoutOptions
	}
	maps.Copy(props, opts.Properties)
	return doc.createAppObject(ctx, opts.ID, props)
}

// Master objects

// MasterObjectOptions are the properties of a new master visualization.
type MasterObjectOptions struct {
	// The object's qInfo.qId. Empty lets the engine choose one.
	ID          string
	Title       string
	Description string
	Tags        []string
	// More properties, such as visualization, set after the ones above
	Properties map[string]any
}

// CreateMasterObject creates a master visualization. It returns ErrIDTaken when opts.ID is set and already taken.
func (doc *Doc) CreateMasterObject(ctx context.Context, opts MasterObjectOptions) (*GenericObject, error) {
	tags := opts.Tags
	if tags == nil {
		tags = []string{}
	}
	props := map[string]any{
		"qInfo":    map[string]any{"qType": "masterobject"},
		"qMetaDef": map[string]any{"title": opts.Title, "description": opts.Description, "tags": tags},
	}
	maps.Copy(props, opts.Properties)
	return doc.createAppObject(ctx, opts.ID, props)
}

// Stories

// Story is a story object, with helpers for its slides.
type Story struct {
	*GenericObject
	doc *Doc
}

// StoryOptions are the properties of a new story.
type StoryOptions struct {
	// The story's qInfo.qId. Empty lets the engine choose one.
	ID          string
	Title       string
	Description string
	// URL of the thumbnail image
	Thumbnail string
	// Position among the stories. nil puts the story first, like a rank of -1.
	Rank *float64
	// More properties, set after the ones above
	Properties map[string]any
}

// CreateStory creates a story with the properties the Qlik Sense client expects.
// It returns ErrIDTaken when opts.ID is set and already taken.
func (doc *Doc) CreateStory(ctx context.Context, opts StoryOptions) (*Story, error) {
	props := map[string]any{
		"qInfo":         map[string]any{"qType": "story"},
		"qMetaDef":      map[string]any{"title": opts.Title, "description": opts.Description},
		"rank":          rankOrLast(opts.Rank),
		"thumbnail":     thumbnailProps(opts.Thumbnail),
		"qChildListDef": map[string]any{"qData": map[string]any{"title": "/title", "rank": "/rank"}},
	}
	maps.Copy(props, opts.Properties)
	obj, err := doc.createAppObject(ctx, opts.ID, props)
	if err != nil {
		return nil, err
	}
	return &Story{GenericObject: obj, doc: doc}, nil
}

// GetStory returns the story with the given id, or an error if the object is not a story.
func (doc *Doc) GetStory(ctx context.Context, id string) (*Story, error) {
	obj, err := doc.GetObject(ctx, id)
	if err != nil {
		return nil, err
	}
	if obj.GenericType != "story" {
		return nil, fmt.Errorf("enigma: object %s is a %s, not a story", id, obj.GenericType)
	}
	return &Story{GenericObject: obj, doc: doc}, nil
}

// Clone creates a copy of the story, including its slides.
func (story *Story) Clone(ctx context.Context) (*Story, error) {
	id, err := story.doc.CloneObject(ctx, story.GenericId)
	if err != nil {
		return nil, err
	}
	return story.doc.GetStory(ctx, id)
}

// Slide is a slide in a story.
type Slide struct {
	*GenericObject
	story *Story
}

// CreateSlide adds a slide to the story. props are set over the default slide properties, for example
// {"rank": 2.5}. Without a rank the slide goes first. The engine chooses the slide's id unless
// props has a qInfo with one; if that id is taken, the engine uses a random one instead.
func (story *Story) CreateSlide(ctx context.Context, props map[string]any) (*Slide, error) {
	slideProps := map[string]any{
		"qInfo": map[string]any{"qType": "slide"},
		"rank":  -1,
		"qChildListDef": map[string]any{"qData": map[string]any{
			"title":             "/title",
			"sheetId":           "/sheetId",
			"ratio":             "/ratio",
			"position":          "/position",
			"dataPath":          "/dataPath",
			"srcPath":           "/srcPath",
			"visualization":     "/visualization",
			"visualizationType": "/visualizationType",
			"style":             "/style",
		}},
	}
	maps.Copy(slideProps, props)
	obj, err := story.CreateChildRaw(ctx, slideProps, nil)
	if err != nil {
		return nil, err
	}
	return &Slide{GenericObject: obj, story: story}, nil
}

// GetSlide returns the slide with the given id in the story.
func (story *Story) GetSlide(ctx context.Context, id string) (*Slide, error) {
	obj, err := story.GetChild(ctx, id)
	if err != nil {
		return nil, err
	}
	return &Slide{GenericObject: obj, story: story}, nil
}

// slideRanks returns the story's slides as id and rank, sorted by rank
func (story *Story) slideRanks(ctx context.Context) ([]slideRank, error) {
	raw, err := story.GetLayoutRaw(ctx)
	if err != nil {
		return nil, err
	}
	var layout struct {
		ChildList struct {
			Items []struct {
				Info NxInfo `json:"qInfo"`
				Data struct {
					Rank Float64 `json:"rank"`
				} `json:"qData"`
			} `json:"qItems"`
		} `json:"qChildList"`
	}
	if err := json.Unmarshal(raw, &layout); err != nil {
		return nil, err
	}
	var slides []slideRank
	for _, item := range layout.ChildList.Items {
		slides = append(slides, slideRank{item.Info.Id, float64(item.Data.Rank)})
	}
	slices.SortStableFunc(slides, func(a, b slideRank) int {
		switch {
		case a.rank < b.rank:
			return -1
		case a.rank > b.rank:
			return 1
		}
		return 0
	})
	return slides, nil
}

type slideRank struct {
	id   string
	rank float64
}

// Duplicate creates a copy of the slide, including its items, right after it in the story.
func (slide *Slide) Duplicate(ctx context.Context) (*Slide, error) {
	slides, err := slide.story.slideRanks(ctx)
	if err != nil {
		return nil, err
	}
	index := slices.IndexFunc(slides, func(s slideRank) bool { return s.id == slide.GenericId })
	if index < 0 {
		return nil, fmt.Errorf("enigma: slide %s is not in story %s", slide.GenericId, slide.story.GenericId)
	}
	// Halfway to the next slide, or one more than the last
	rank := slides[index].rank + 1
	if index+1 < len(slides) {
		rank = (slides[index].rank + slides[index+1].rank) / 2
	}
	duplicate, err := slide.story.CreateSlide(ctx, map[string]any{"rank": rank})
	if err != nil {
		return nil, err
	}
	if err := duplicate.CopyFrom(ctx, slide.GenericId); err != nil {
		return nil, err
	}
	// CopyFrom also copied the rank
	raw, err := duplicate.GetPropertiesRaw(ctx)
	if err != nil {
		return nil, err
	}
	var props map[string]any
	if err := json.Unmarshal(raw, &props); err != nil {
		return nil, err
	}
	props["rank"] = rank
	if err := duplicate.SetPropertiesRaw(ctx, props); err != nil {
		return nil, err
	}
	return duplicate, nil
}
