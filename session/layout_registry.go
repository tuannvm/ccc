package session

// BuiltinLayouts contains all built-in layout specifications
var BuiltinLayouts = map[string]LayoutSpec{
	"single": {
		Name: "single",
		Panes: []PaneSpec{
			{
				ID:        "standard",
				Index:     0,
				DefaultIn: true,
			},
		},
	},
	"team-3pane": {
		Name: "team-3pane",
		Panes: []PaneSpec{
			{
				ID:        "planner",
				Index:     0,
				DefaultIn: false,
				Prefixes:  []string{"/planner", "/plan", "@planner"},
			},
			{
				ID:        "executor",
				Index:     1,
				DefaultIn: true,
				Prefixes:  []string{"/executor", "/exec", "/e", "@executor"},
			},
			{
				ID:        "reviewer",
				Index:     2,
				DefaultIn: false,
				Prefixes:  []string{"/reviewer", "/rev", "/r", "@reviewer"},
			},
		},
	},
	// Future extensibility examples:
	// "team-4pane": {
	// 	Name: "team-4pane",
	// 	Panes: []PaneSpec{
	// 		{ID: "planner", Index: 0, Prefixes: []string{"/p", "/planner"}},
	// 		{ID: "executor", Index: 1, Prefixes: []string{"/e", "/executor"}},
	// 		{ID: "reviewer", Index: 2, Prefixes: []string{"/r", "/reviewer"}},
	// 		{ID: "observer", Index: 3, Prefixes: []string{"/o", "/observer"}},
	// 	},
	// },
	// "grid-2x2": {
	// 	Name: "grid-2x2",
	// 	Panes: []PaneSpec{
	// 		{ID: "top-left", Index: 0},
	// 		{ID: "top-right", Index: 1},
	// 		{ID: "bottom-left", Index: 2},
	// 		{ID: "bottom-right", Index: 3},
	// 	},
	// },
}

// GetLayout retrieves a layout specification by name
// Returns the layout and true if found, nil and false otherwise
func GetLayout(name string) (LayoutSpec, bool) {
	layout, ok := BuiltinLayouts[name]
	return layout, ok
}

// GetDefaultLayout returns the default layout specification (single-pane)
func GetDefaultLayout() LayoutSpec {
	return BuiltinLayouts["single"]
}
