package staticfiles_test

import (
	"testing"

	"github.com/oliverhaas/djangogo/djangotest"
	"github.com/oliverhaas/djangogo/staticfiles"
)

// The tests in this file are ports of Django's
// tests/staticfiles_tests/test_finders.py. Django has one finder class per
// kind of location; here one Finder searches an ordered list of sources, so a
// Django finder becomes a Finder over the sources of that kind.
//
// Not ported, because the feature has no counterpart: TestBaseFinder (an
// abstract base class), the get_finder tests of TestMiscFinder (finders named
// by Python import strings and their cache), and
// TestMiscFinder.test_location_empty (a storage finder without a location).

// locations returns the Location of every match, which is what Django's
// finders return in place of a Match.
func locations(matches []staticfiles.Match) []string {
	found := make([]string, 0, len(matches))
	for _, m := range matches {
		found = append(found, m.Location)
	}
	return found
}

// roots returns the directory of every source, the counterpart of Django's
// finders.searched_locations.
func roots(sources []staticfiles.Source) []string {
	searched := make([]string, 0, len(sources))
	for _, src := range sources {
		searched = append(searched, src.Root)
	}
	return searched
}

// Ported from Django: tests/staticfiles_tests/test_finders.py::TestFileSystemFinder.test_find_first
func TestFileSystemFinderFindFirst(t *testing.T) {
	s := newSite(t)
	finder := staticfiles.NewFinder(s.dirs...)

	found, ok := finder.Find("test/file.txt")

	djangotest.AssertEqual(t, ok, true)
	djangotest.AssertEqual(t, found.Location, s.path("documents", "test", "file.txt"))
	djangotest.AssertEqual(t, found.Path, "test/file.txt")
}

// Ported from Django: tests/staticfiles_tests/test_finders.py::TestFileSystemFinder.test_find_all
func TestFileSystemFinderFindAll(t *testing.T) {
	s := newSite(t)
	finder := staticfiles.NewFinder(s.dirs...)

	found := finder.FindAll("test/file.txt")

	djangotest.AssertSequenceEqual(t, locations(found), []string{s.path("documents", "test", "file.txt")})
}

// TestAppDirectoriesFinderFindFirst finds a file in the static files of an
// app. The app is an in-memory filesystem, so the location names the app and
// the path inside it where Django has an absolute path.
//
// Ported from Django: tests/staticfiles_tests/test_finders.py::TestAppDirectoriesFinder.test_find_first
func TestAppDirectoriesFinderFindFirst(t *testing.T) {
	s := newSite(t)
	finder := staticfiles.NewFinder(s.apps...)

	found, ok := finder.Find("test/file1.txt")

	djangotest.AssertEqual(t, ok, true)
	djangotest.AssertEqual(t, found.Location, "test:test/file1.txt")
	djangotest.AssertEqual(t, found.Source.Name, "test")
}

// Ported from Django: tests/staticfiles_tests/test_finders.py::TestAppDirectoriesFinder.test_find_all
func TestAppDirectoriesFinderFindAll(t *testing.T) {
	s := newSite(t)
	finder := staticfiles.NewFinder(s.apps...)

	found := finder.FindAll("test/file1.txt")

	djangotest.AssertSequenceEqual(t, locations(found), []string{"test:test/file1.txt"})
}

// TestDefaultStorageFinderFindFirst finds a file in the media root, which is
// one more directory source here.
//
// Ported from Django: tests/staticfiles_tests/test_finders.py::TestDefaultStorageFinder.test_find_first
func TestDefaultStorageFinderFindFirst(t *testing.T) {
	s := newSite(t)
	finder := staticfiles.NewFinder(s.media)

	found, ok := finder.Find("media-file.txt")

	djangotest.AssertEqual(t, ok, true)
	djangotest.AssertEqual(t, found.Location, s.path("site_media", "media", "media-file.txt"))
}

// Ported from Django: tests/staticfiles_tests/test_finders.py::TestDefaultStorageFinder.test_find_all
func TestDefaultStorageFinderFindAll(t *testing.T) {
	s := newSite(t)
	finder := staticfiles.NewFinder(s.media)

	found := finder.FindAll("media-file.txt")

	djangotest.AssertSequenceEqual(t, locations(found), []string{s.path("site_media", "media", "media-file.txt")})
}

// TestSearchedLocations looks a missing file up. Django records the searched
// directories in the module-level list finders.searched_locations; a Finder
// has no such state, and its Sources are the locations it searches.
//
// Ported from Django: tests/staticfiles_tests/test_finders.py::TestMiscFinder.test_searched_locations
func TestSearchedLocations(t *testing.T) {
	s := newSite(t)
	finder := staticfiles.NewFinder(staticfiles.DirSource(s.path("documents")))

	_, ok := finder.Find("spam")

	djangotest.AssertEqual(t, ok, false)
	djangotest.AssertSequenceEqual(t, roots(finder.Sources()), []string{s.path("documents")})
}

// Ported from Django: tests/staticfiles_tests/test_finders.py::TestMiscFinder.test_searched_locations_find_all
func TestSearchedLocationsFindAll(t *testing.T) {
	s := newSite(t)
	finder := staticfiles.NewFinder(staticfiles.DirSource(s.path("documents")))

	found := finder.FindAll("spam")

	djangotest.AssertEqual(t, len(found), 0)
	djangotest.AssertSequenceEqual(t, roots(finder.Sources()), []string{s.path("documents")})
}
