package tmdb

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
)

func TestMovieCollections_ParseAuthoritativeReferenceAndRoster(t *testing.T) {
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: testResponse(`{
			"id":671,
			"belongs_to_collection":{"id":1241,"name":"Harry Potter Collection"}
		}`)},
		httpfixture.Step{Response: testResponse(`{
			"id":1241,
			"name":"Harry Potter Collection",
			"parts":[
				{"id":671,"title":"Harry Potter and the Philosopher's Stone","release_date":"2001-11-16","genre_ids":[12,14],"overview":"A young wizard begins school.","original_language":"en","vote_average":7.9,"vote_count":28000},
				{"id":672,"title":"Harry Potter and the Chamber of Secrets","release_date":"2002-11-13","genre_ids":[12,14],"overview":"The second school year.","original_language":"en","vote_average":7.7,"vote_count":22000}
			]
		}`)},
	)
	client := testClient(func() string { return "key" }, script)

	ref, err := client.CollectionForMovie(context.Background(), 671)
	if err != nil {
		t.Fatal(err)
	}
	if ref.TMDBID != 1241 || ref.Name != "Harry Potter Collection" {
		t.Fatalf("reference = %+v", ref)
	}
	collection, err := client.MovieCollection(context.Background(), ref.TMDBID)
	if err != nil {
		t.Fatal(err)
	}
	if collection.TMDBID != 1241 || collection.Name != "Harry Potter Collection" || len(collection.Members) != 2 {
		t.Fatalf("collection = %+v", collection)
	}
	first := collection.Members[0]
	if first.TMDBID != 671 || first.Name != "Harry Potter and the Philosopher's Stone" || first.Year != 2001 {
		t.Fatalf("first member identity = %+v", first)
	}
	if len(first.Genres) != 2 || first.Genres[0] != "Adventure" || first.Genres[1] != "Fantasy" || first.Overview == "" {
		t.Fatalf("first member evidence = %+v", first)
	}
	requests := script.Requests()
	if len(requests) != 2 || requests[0].URL != testBaseURL+"/movie/671" || requests[1].URL != testBaseURL+"/collection/1241" {
		t.Fatalf("requests = %+v", requests)
	}
}

func TestCollectionForMovie_ReturnsAuthoritativeStandalone(t *testing.T) {
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: testResponse(`{"id":550,"belongs_to_collection":null}`)},
	)
	client := testClient(func() string { return "key" }, script)

	ref, err := client.CollectionForMovie(context.Background(), 550)
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
			"id":99,
			"name":"Example Collection",
			"parts":[
				{"id":20,"title":"December Film","release_date":"2001-12-14"},
				{"id":30,"title":"February Film","release_date":"2001-02-02"}
			]
		}`)},
	)
	client := testClient(func() string { return "key" }, script)

	collection, err := client.MovieCollection(context.Background(), 99)
	if err != nil {
		t.Fatal(err)
	}
	if len(collection.Members) != 2 || collection.Members[0].TMDBID != 30 || collection.Members[1].TMDBID != 20 {
		t.Fatalf("members = %+v, want exact release-date order within 2001", collection.Members)
	}
}
