package api

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// The upload's ownerKind enum is the store's list of image owner tables (#2793): a kind the store
// has no table for would pass validation and then fail the upload.
func TestUploadOwnerKindEnumIsTheStoresOwnerTables(t *testing.T) {
	field, ok := reflect.TypeFor[uploadImageInput]().FieldByName("OwnerKind")
	if !ok {
		t.Fatal("uploadImageInput has no OwnerKind")
	}
	got := strings.Split(field.Tag.Get("enum"), ",")
	if want := clipstore.ImageOwnerKinds(); !slices.Equal(got, want) {
		t.Errorf("ownerKind enum = %v, want clipstore.ImageOwnerKinds() = %v", got, want)
	}
}
