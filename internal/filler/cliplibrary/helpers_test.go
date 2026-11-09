package cliplibrary_test

import "github.com/loomarr/loomarr/internal/filler/cliplibrary"

func testLayout(root string, watch ...string) cliplibrary.Layout {
	configuredWatch := ""
	if len(watch) > 0 {
		configuredWatch = watch[0]
	}
	layout, err := cliplibrary.NewLayout(root, configuredWatch)
	if err != nil {
		panic(err)
	}
	return layout
}
