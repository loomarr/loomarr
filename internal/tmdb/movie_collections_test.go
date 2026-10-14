package tmdb

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
)

func TestMovieCollections_ParseAuthoritativeReferenceAndRoster(t *testing.T) {
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: testResponse(`{
			"id":90000671,
			"belongs_to_collection":{"id":90001241,"name":"Wren Halloway Collection"}
		}`)},
		httpfixture.Step{Response: testResponse(`{
			"id":90001241,
			"name":"Wren Halloway Collection",
			"parts":[
				{"id":90000671,"title":"Wren Halloway and the Lantern Key","release_date":"2001-11-16","genre_ids":[12,14],"overview":"A young archivist begins school.","original_language":"en","vote_average":7.9,"vote_count":28000},
				{"id":90000672,"title":"Wren Halloway and the Sunken Library","release_date":"2002-11-13","genre_ids":[12,14],"overview":"The second school year.","original_language":"en","vote_average":7.7,"vote_count":22000}
			]
		}`)},
	)
	client := testClient(func() string { return "key" }, script)

	ref, err := client.CollectionForMovie(context.Background(), 90000671)
	if err != nil {
		t.Fatal(err)
	}
	if ref.TMDBID != 90001241 || ref.Name != "Wren Halloway Collection" {
		t.Fatalf("reference = %+v", ref)
	}
	collection, err := client.MovieCollection(context.Background(), ref.TMDBID)
	if err != nil {
		t.Fatal(err)
	}
	if collection.TMDBID != 90001241 || collection.Name != "Wren Halloway Collection" || len(collection.Members) != 2 {
		t.Fatalf("collection = %+v", collection)
	}
	first := collection.Members[0]
	if first.TMDBID != 90000671 || first.Name != "Wren Halloway and the Lantern Key" || first.Year != 2001 {
		t.Fatalf("first member identity = %+v", first)
	}
	if len(first.Genres) != 2 || first.Genres[0] != "Adventure" || first.Genres[1] != "Fantasy" || first.Overview == "" {
		t.Fatalf("first member evidence = %+v", first)
	}
	requests := script.Requests()
	if len(requests) != 2 || requests[0].URL != testBaseURL+"/movie/90000671" || requests[1].URL != testBaseURL+"/collection/90001241" {
		t.Fatalf("requests = %+v", requests)
	}
}

func TestCollectionForMovie_ReturnsAuthoritativeStandalone(t *testing.T) {
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: testResponse(`{"id":90000550,"belongs_to_collection":null}`)},
	)
	client := testClient(func() string { return "key" }, script)

	ref, err := client.CollectionForMovie(context.Background(), 90000550)
	if err != nil {
		t.Fatal(err)
	}
	if ref.TMDBID != 0 || ref.Name != "" {
		t.Fatalf("reference = %+v, want authoritative standalone", ref)
	}
}

func TestMovieCollection_OrdersSameYearMembersByReleaseDate(t *testing.T) {
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: testResponse(`{
			"id":90000099,
			"name":"Example Collection",
			"parts":[
				{"id":90000020,"title":"December Film","release_date":"2001-12-14"},
				{"id":90000030,"title":"February Film","release_date":"2001-02-02"}
			]
		}`)},
	)
	client := testClient(func() string { return "key" }, script)

	collection, err := client.MovieCollection(context.Background(), 90000099)
	if err != nil {
		t.Fatal(err)
	}
	if len(collection.Members) != 2 || collection.Members[0].TMDBID != 90000030 || collection.Members[1].TMDBID != 90000020 {
		t.Fatalf("members = %+v, want exact release-date order within 2001", collection.Members)
	}
}
